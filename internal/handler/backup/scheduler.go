package backup

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Scheduler runs active BackupSchedule cron expressions and triggers backups.
type Scheduler struct {
	db      *gorm.DB
	cron    *cron.Cron
	logger  *zap.Logger
	mu      sync.Mutex
	entries map[uint]cron.EntryID
	runner  ScheduleRunner
}

// ScheduleRunner executes a scheduled backup for one BackupSchedule.
type ScheduleRunner interface {
	RunScheduledBackup(schedule *model.BackupSchedule)
}

func NewScheduler(db *gorm.DB, logger *zap.Logger, runner ScheduleRunner) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Scheduler{
		db:      db,
		cron:    cron.New(),
		logger:  logger,
		entries: make(map[uint]cron.EntryID),
		runner:  runner,
	}
}

func (s *Scheduler) Start() error {
	var schedules []model.BackupSchedule
	if err := s.db.Where("status = ?", "active").Find(&schedules).Error; err != nil {
		return fmt.Errorf("list backup schedules: %w", err)
	}
	for i := range schedules {
			if strings.TrimSpace(schedules[i].Schedule) == "" {
				continue
			}
			if err := s.Add(schedules[i]); err != nil {
				s.logger.Warn("skip invalid backup schedule",
					zap.Uint("id", schedules[i].ID),
					zap.String("schedule", schedules[i].Schedule),
					zap.Error(err),
				)
			}
		}
	s.cron.Start()
	s.logger.Info("backup scheduler started", zap.Int("active", len(s.entries)))
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.mu.Lock()
	s.entries = make(map[uint]cron.EntryID)
	s.mu.Unlock()
	s.logger.Info("backup scheduler stopped")
}

// Sync registers or unregisters a schedule based on status and cron expression.
func (s *Scheduler) Sync(schedule model.BackupSchedule) error {
	if schedule.Status != "active" || strings.TrimSpace(schedule.Schedule) == "" {
		s.Remove(schedule.ID)
		return nil
	}
	return s.Add(schedule)
}

func (s *Scheduler) Add(schedule model.BackupSchedule) error {
	if schedule.Status != "" && schedule.Status != "active" {
		s.Remove(schedule.ID)
		return nil
	}
	expr := strings.TrimSpace(schedule.Schedule)
	if expr == "" {
		s.Remove(schedule.ID)
		return fmt.Errorf("empty cron expression")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[schedule.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, schedule.ID)
	}

	id := schedule.ID
	entryID, err := s.cron.AddFunc(expr, func() {
		s.trigger(id)
	})
	if err != nil {
		return err
	}
	s.entries[id] = entryID
	s.logger.Info("backup schedule registered",
		zap.Uint("id", id),
		zap.String("cron", expr),
	)
	return nil
}

func (s *Scheduler) Remove(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, id)
		s.logger.Info("backup schedule unregistered", zap.Uint("id", id))
	}
}

func (s *Scheduler) trigger(scheduleID uint) {
	if s.db == nil || s.runner == nil {
		return
	}
	var schedule model.BackupSchedule
	if err := s.db.First(&schedule, scheduleID).Error; err != nil {
		s.logger.Warn("scheduled backup missing", zap.Uint("id", scheduleID), zap.Error(err))
		return
	}
		if schedule.Status != "active" || strings.TrimSpace(schedule.Schedule) == "" {
			s.Remove(scheduleID)
			return
		}
	s.logger.Info("running scheduled backup",
		zap.Uint("id", schedule.ID),
		zap.String("name", schedule.Name),
	)
	s.runner.RunScheduledBackup(&schedule)

	now := time.Now()
	_ = s.db.Model(&schedule).Update("last_backup", now).Error
}

// ActiveCount returns how many cron entries are currently registered.
func (s *Scheduler) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// DB exposes the scheduler database handle for status aggregation.
func (s *Scheduler) DB() *gorm.DB {
	return s.db
}

// ValidateCron reports whether expr is accepted by the backup scheduler.
func ValidateCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := parser.Parse(expr)
	return err
}
