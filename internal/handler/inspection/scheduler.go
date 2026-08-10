package inspection

import (
	"fmt"
	"sync"
	"strings"

	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Scheduler runs enabled InspectionRule cron expressions.
type Scheduler struct {
	db      *gorm.DB
	cron    *cron.Cron
	logger  *zap.Logger
	mu      sync.Mutex
	entries map[uint]cron.EntryID
	runner  ScheduleRunner
}

// ScheduleRunner executes a scheduled inspection for one rule.
type ScheduleRunner interface {
	RunScheduledInspection(rule *model.InspectionRule)
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
	var rules []model.InspectionRule
	if err := s.db.Where("enabled = ? AND schedule <> ? AND schedule IS NOT NULL", true, "").Find(&rules).Error; err != nil {
		return fmt.Errorf("list inspection schedules: %w", err)
	}
	for i := range rules {
		if err := s.Add(rules[i]); err != nil {
			s.logger.Warn("skip invalid inspection schedule",
				zap.Uint("id", rules[i].ID),
				zap.String("schedule", rules[i].Schedule),
				zap.Error(err),
			)
		}
	}
	s.cron.Start()
	s.logger.Info("inspection scheduler started", zap.Int("active", len(s.entries)))
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.mu.Lock()
	s.entries = make(map[uint]cron.EntryID)
	s.mu.Unlock()
	s.logger.Info("inspection scheduler stopped")
}

func (s *Scheduler) Sync(rule model.InspectionRule) error {
	if !rule.Enabled || strings.TrimSpace(rule.Schedule) == "" {
		s.Remove(rule.ID)
		return nil
	}
	return s.Add(rule)
}

func (s *Scheduler) Add(rule model.InspectionRule) error {
	expr := strings.TrimSpace(rule.Schedule)
	if expr == "" {
		s.Remove(rule.ID)
		return nil
	}
	if !rule.Enabled {
		s.Remove(rule.ID)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[rule.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, rule.ID)
	}

	id := rule.ID
	entryID, err := s.cron.AddFunc(expr, func() {
		s.trigger(id)
	})
	if err != nil {
		return err
	}
	s.entries[id] = entryID
	s.logger.Info("inspection schedule registered",
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
		s.logger.Info("inspection schedule unregistered", zap.Uint("id", id))
	}
}

func (s *Scheduler) trigger(ruleID uint) {
	if s.db == nil || s.runner == nil {
		return
	}
	var rule model.InspectionRule
	if err := s.db.First(&rule, ruleID).Error; err != nil {
		s.logger.Warn("scheduled inspection missing", zap.Uint("id", ruleID), zap.Error(err))
		return
	}
	if !rule.Enabled || strings.TrimSpace(rule.Schedule) == "" {
		s.Remove(ruleID)
		return
	}
	s.logger.Info("running scheduled inspection",
		zap.Uint("id", rule.ID),
		zap.String("name", rule.Name),
	)
	s.runner.RunScheduledInspection(&rule)
}

func (s *Scheduler) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// DB exposes the scheduler database handle for status aggregation.
func (s *Scheduler) DB() *gorm.DB {
	return s.db
}

// ValidateCron reports whether expr is accepted by the inspection scheduler.
func ValidateCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := parser.Parse(expr)
	return err
}
