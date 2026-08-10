package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	schedulerHandler "github.com/kubepilot/kubepilot/internal/handler/scheduler"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"gorm.io/gorm"
)

type Module struct {
	module.Base
	db *gorm.DB
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Metadata {
	return module.Metadata{
		Name:          "scheduler",
		Version:       "1.0.0",
		Description:   "Task queues, jobs, and resource reservations",
		MultiInstance: module.MultiInstanceAll,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.TaskQueue{},
		&model.Task{},
		&model.TaskLog{},
		&model.ResourceReservation{},
		&model.SchedulePolicy{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "scheduler", Actions: []string{"view", "create", "edit", "delete", "execute"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	return []module.MenuItem{
		{Key: "/scheduler", Label: "任务调度", Order: 80, Resource: "scheduler", Action: "view"},
		{Key: "/scheduler/tasks", Label: "任务管理", Parent: "/scheduler", Order: 1, Resource: "scheduler", Action: "view"},
		{Key: "/scheduler/queues", Label: "队列管理", Parent: "/scheduler", Order: 2, Resource: "scheduler", Action: "view"},
	}
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	reg.MustRegister("GET", "/api/v1/scheduler/queues", authz.Policy{Resource: "scheduler", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/scheduler/queues", authz.Policy{Resource: "scheduler", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/scheduler/queues/:id", authz.Policy{Resource: "scheduler", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("PUT", "/api/v1/scheduler/queues/:id", authz.Policy{Resource: "scheduler", Action: "edit", Scope: authz.ScopePlatform})
	reg.MustRegister("DELETE", "/api/v1/scheduler/queues/:id", authz.Policy{Resource: "scheduler", Action: "delete", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/scheduler/tasks", authz.Policy{Resource: "scheduler", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/scheduler/tasks", authz.Policy{Resource: "scheduler", Action: "create", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/scheduler/tasks/:id", authz.Policy{Resource: "scheduler", Action: "view", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/scheduler/tasks/:id/cancel", authz.Policy{Resource: "scheduler", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/scheduler/tasks/:id/retry", authz.Policy{Resource: "scheduler", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("DELETE", "/api/v1/scheduler/tasks/:id", authz.Policy{Resource: "scheduler", Action: "delete", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/scheduler/tasks/:id/logs", authz.Policy{Resource: "scheduler", Action: "view", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/scheduler/reservations", authz.Policy{Resource: "scheduler", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/scheduler/reservations", authz.Policy{Resource: "scheduler", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("DELETE", "/api/v1/scheduler/reservations/:id", authz.Policy{Resource: "scheduler", Action: "delete", Scope: authz.ScopePlatform})
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	m.db = host.DB
	return nil
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.db == nil {
		return nil
	}
	var queues, running, failedRecent int64
	_ = m.db.Model(&model.TaskQueue{}).Count(&queues).Error
	_ = m.db.Model(&model.Task{}).Where("status = ?", "running").Count(&running).Error
	_ = m.db.Model(&model.Task{}).Where("status = ? AND created_at > ?", "failed", time.Now().Add(-24*time.Hour)).Count(&failedRecent).Error
	details := map[string]any{
		"queues":           queues,
		"running_tasks":    running,
		"failed_tasks_24h": failedRecent,
	}
	if failedRecent > 0 {
		details["health_warning"] = fmt.Sprintf("近24小时失败任务 %d 个", failedRecent)
	}
	return details
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	if m.db == nil {
		m.db = ctx.Host.DB
	}
	h := schedulerHandler.NewHandler(ctx.Host.DB)
	g := protected.Group("/scheduler")
	{
		g.GET("/queues", h.ListQueues)
		g.POST("/queues", h.CreateQueue)
		g.GET("/queues/:id", h.GetQueue)
		g.PUT("/queues/:id", h.UpdateQueue)
		g.DELETE("/queues/:id", h.DeleteQueue)
		g.GET("/tasks", h.ListTasks)
		g.POST("/tasks", h.CreateTask)
		g.GET("/tasks/:id", h.GetTask)
		g.POST("/tasks/:id/cancel", h.CancelTask)
		g.POST("/tasks/:id/retry", h.RetryTask)
		g.DELETE("/tasks/:id", h.DeleteTask)
		g.GET("/tasks/:id/logs", h.GetTaskLogs)
		g.GET("/reservations", h.ListReservations)
		g.POST("/reservations", h.CreateReservation)
		g.DELETE("/reservations/:id", h.DeleteReservation)
	}
}
