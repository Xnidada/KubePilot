package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	backupHandler "github.com/kubepilot/kubepilot/internal/handler/backup"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"go.uber.org/zap"
)

type Module struct {
	module.Base
	handler   *backupHandler.Handler
	scheduler *backupHandler.Scheduler
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Metadata {
	return module.Metadata{
		Name:          "backup",
		Version:       "1.0.0",
		Description:   "Backup schedules, records, and restores",
		MultiInstance: module.MultiInstanceLeaderOnly,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.BackupSchedule{},
		&model.BackupRecord{},
		&model.RestoreRecord{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "backups", Actions: []string{"view", "create", "delete", "execute"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	return []module.MenuItem{
		{Key: "/system/backup", Label: "备份管理", Parent: "/system", Order: 50, Resource: "backups", Action: "view"},
	}
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	reg.MustRegister("GET", "/api/v1/backups", authz.Policy{Resource: "backups", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/backups", authz.Policy{Resource: "backups", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/backups/:id", authz.Policy{Resource: "backups", Action: "view", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/backups/schedules", authz.Policy{Resource: "backups", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/backups/schedules", authz.Policy{Resource: "backups", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("PUT", "/api/v1/backups/schedules/:id", authz.Policy{Resource: "backups", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("DELETE", "/api/v1/backups/schedules/:id", authz.Policy{Resource: "backups", Action: "delete", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/backups/schedules/:id/pause", authz.Policy{Resource: "backups", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/backups/schedules/:id/resume", authz.Policy{Resource: "backups", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("DELETE", "/api/v1/backups/schedules/:id/cron", authz.Policy{Resource: "backups", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/backups/restore", authz.Policy{Resource: "backups", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/backups/restores", authz.Policy{Resource: "backups", Action: "view", Scope: authz.ScopePlatform})
}

func (m *Module) ensureHandler(host *module.Host) *backupHandler.Handler {
	if m.handler == nil {
		m.handler = backupHandler.NewHandler(host.DB)
	}
	return m.handler
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	h := m.ensureHandler(host)
	logger := host.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	m.scheduler = backupHandler.NewScheduler(host.DB, logger, h)
	h.SetScheduler(m.scheduler)
	return m.scheduler.Start()
}

func (m *Module) Stop(ctx context.Context) error {
	if m.scheduler != nil {
		m.scheduler.Stop()
		m.scheduler = nil
	}
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.scheduler == nil {
		return fmt.Errorf("backup scheduler not started")
	}
	return nil
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.scheduler == nil {
		return nil
	}
	var active, paused, failedRecent int64
	db := m.scheduler.DB()
	if db != nil {
		_ = db.Model(&model.BackupSchedule{}).Where("status = ?", "active").Count(&active).Error
		_ = db.Model(&model.BackupSchedule{}).Where("status = ?", "paused").Count(&paused).Error
		_ = db.Model(&model.BackupRecord{}).Where("status = ? AND created_at > ?", "failed", time.Now().Add(-24*time.Hour)).Count(&failedRecent).Error
	}
	details := map[string]any{
		"cron_entries":       m.scheduler.ActiveCount(),
		"active_schedules":   active,
		"paused_schedules":   paused,
		"failed_backups_24h": failedRecent,
	}
	if failedRecent > 0 {
		details["health_warning"] = fmt.Sprintf("近24小时失败备份 %d 次", failedRecent)
	}
	return details
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	h := m.ensureHandler(ctx.Host)
	if m.scheduler != nil {
		h.SetScheduler(m.scheduler)
	}
	g := protected.Group("/backups")
	{
		g.GET("/schedules", h.ListBackupSchedules)
		g.POST("/schedules", h.CreateBackupSchedule)
		g.PUT("/schedules/:id", h.UpdateBackupSchedule)
		g.DELETE("/schedules/:id", h.DeleteBackupSchedule)
		g.POST("/schedules/:id/pause", h.PauseBackupSchedule)
		g.POST("/schedules/:id/resume", h.ResumeBackupSchedule)
		g.DELETE("/schedules/:id/cron", h.ClearBackupCron)
		g.POST("", h.CreateBackup)
		g.GET("", h.ListBackupRecords)
		g.GET("/:id", h.GetBackupRecord)
		g.POST("/restore", h.CreateRestore)
		g.GET("/restores", h.ListRestoreRecords)
	}
}
