package inspection

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	inspectionHandler "github.com/kubepilot/kubepilot/internal/handler/inspection"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"go.uber.org/zap"
)

type Module struct {
	module.Base
	handler   *inspectionHandler.InspectionHandler
	scheduler *inspectionHandler.Scheduler
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Metadata {
	return module.Metadata{
		Name:          "inspection",
		Version:       "1.0.0",
		Description:   "Cluster inspection rules and reports",
		MultiInstance: module.MultiInstanceLeaderOnly,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.InspectionRule{},
		&model.InspectionReport{},
		&model.InspectionResult{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "inspection", Actions: []string{"view", "create", "edit", "delete", "execute"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	return []module.MenuItem{
		{Key: "/cluster/inspection", Label: "集群巡检", Parent: "/ops", Order: 10, Resource: "inspection", Action: "view"},
	}
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	reg.MustRegister("GET", "/api/v1/inspection/rules", authz.Policy{Resource: "inspection", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/inspection/rules", authz.Policy{Resource: "inspection", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/inspection/rules/:id", authz.Policy{Resource: "inspection", Action: "view", Scope: authz.ScopeHandler})
	reg.MustRegister("PUT", "/api/v1/inspection/rules/:id", authz.Policy{Resource: "inspection", Action: "edit", Scope: authz.ScopeHandler})
	reg.MustRegister("DELETE", "/api/v1/inspection/rules/:id", authz.Policy{Resource: "inspection", Action: "delete", Scope: authz.ScopeHandler})
	reg.MustRegister("DELETE", "/api/v1/inspection/rules/:id/schedule", authz.Policy{Resource: "inspection", Action: "edit", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/inspection/rules/:id/run", authz.Policy{Resource: "inspection", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/inspection/reports", authz.Policy{Resource: "inspection", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/inspection/reports/:id", authz.Policy{Resource: "inspection", Action: "view", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/inspection/reports/:id/results", authz.Policy{Resource: "inspection", Action: "view", Scope: authz.ScopeHandler})
}

func (m *Module) ensureHandler(host *module.Host) *inspectionHandler.InspectionHandler {
	if m.handler == nil {
		m.handler = inspectionHandler.NewInspectionHandler(host.DB)
	}
	return m.handler
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	h := m.ensureHandler(host)
	logger := host.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	m.scheduler = inspectionHandler.NewScheduler(host.DB, logger, h)
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
		return fmt.Errorf("inspection scheduler not started")
	}
	return nil
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.scheduler == nil {
		return nil
	}
	var enabled, scheduled, failedRecent, running int64
	db := m.scheduler.DB()
	if db != nil {
		_ = db.Model(&model.InspectionRule{}).Where("enabled = ?", true).Count(&enabled).Error
		_ = db.Model(&model.InspectionRule{}).Where("enabled = ? AND schedule <> ? AND schedule IS NOT NULL", true, "").Count(&scheduled).Error
		_ = db.Model(&model.InspectionReport{}).Where("status = ?", "running").Count(&running).Error
		_ = db.Model(&model.InspectionReport{}).Where("status = ? AND created_at > ?", "failed", time.Now().Add(-24*time.Hour)).Count(&failedRecent).Error
	}
	details := map[string]any{
		"cron_entries":       m.scheduler.ActiveCount(),
		"enabled_rules":      enabled,
		"scheduled_rules":    scheduled,
		"running_reports":    running,
		"failed_reports_24h": failedRecent,
	}
	if failedRecent > 0 {
		details["health_warning"] = fmt.Sprintf("近24小时失败巡检报告 %d 份", failedRecent)
	}
	return details
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	h := m.ensureHandler(ctx.Host)
	if m.scheduler != nil {
		h.SetScheduler(m.scheduler)
	}
	g := protected.Group("/inspection")
	{
		g.GET("/rules", h.ListRules)
		g.POST("/rules", h.CreateRule)
		g.GET("/rules/:id", h.GetRule)
		g.PUT("/rules/:id", h.UpdateRule)
		g.DELETE("/rules/:id", h.DeleteRule)
		g.DELETE("/rules/:id/schedule", h.ClearSchedule)
		g.POST("/rules/:id/run", h.RunInspection)
		g.GET("/reports", h.ListReports)
		g.GET("/reports/:id", h.GetReport)
		g.GET("/reports/:id/results", h.GetReportResults)
	}
}
