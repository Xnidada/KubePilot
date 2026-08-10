package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	whHandler "github.com/kubepilot/kubepilot/internal/handler/webhook"
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
		Name:          "webhook",
		Version:       "1.0.0",
		Description:   "Outbound webhook notifications",
		MultiInstance: module.MultiInstanceAll,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.WebhookConfig{},
		&model.WebhookLog{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "webhooks", Actions: []string{"view", "create", "edit", "delete", "execute"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	return []module.MenuItem{
		{Key: "/system/webhooks", Label: "Webhook 通知", Parent: "/system", Order: 60, Resource: "webhooks", Action: "view"},
	}
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	reg.MustRegister("GET", "/api/v1/webhooks", authz.Policy{Resource: "webhooks", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/webhooks", authz.Policy{Resource: "webhooks", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("PUT", "/api/v1/webhooks/:id", authz.Policy{Resource: "webhooks", Action: "edit", Scope: authz.ScopePlatform})
	reg.MustRegister("DELETE", "/api/v1/webhooks/:id", authz.Policy{Resource: "webhooks", Action: "delete", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/webhooks/:id/test", authz.Policy{Resource: "webhooks", Action: "execute", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/webhooks/logs", authz.Policy{Resource: "webhooks", Action: "view", Scope: authz.ScopePlatform})
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	m.db = host.DB
	return nil
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.db == nil {
		return nil
	}
	var enabled, failedRecent, okRecent int64
	since := time.Now().Add(-24 * time.Hour)
	_ = m.db.Model(&model.WebhookConfig{}).Where("enabled = ?", true).Count(&enabled).Error
	_ = m.db.Model(&model.WebhookLog{}).Where("status = ? AND created_at > ?", "failed", since).Count(&failedRecent).Error
	_ = m.db.Model(&model.WebhookLog{}).Where("status = ? AND created_at > ?", "success", since).Count(&okRecent).Error
	details := map[string]any{
		"enabled_webhooks":      enabled,
		"failed_deliveries_24h": failedRecent,
		"ok_deliveries_24h":     okRecent,
	}
	if failedRecent >= 5 {
		details["health_warning"] = fmt.Sprintf("近24小时 Webhook 投递失败 %d 次", failedRecent)
	}
	return details
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	if m.db == nil {
		m.db = ctx.Host.DB
	}
	h := whHandler.NewHandler(ctx.Host.DB)
	g := protected.Group("/webhooks")
	{
		g.GET("", h.ListWebhooks)
		g.POST("", h.CreateWebhook)
		g.PUT("/:id", h.UpdateWebhook)
		g.DELETE("/:id", h.DeleteWebhook)
		g.POST("/:id/test", h.TestWebhook)
		g.GET("/logs", h.ListWebhookLogs)
	}
}
