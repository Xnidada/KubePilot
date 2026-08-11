package appstore

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"gorm.io/gorm"
)

// Module owns appstore models and menu metadata. HTTP APIs are not implemented yet.
type Module struct {
	module.Base
	db *gorm.DB
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Metadata {
	return module.Metadata{
		Name:          "appstore",
		Version:       "0.1.0",
		Description:   "Application store (Helm templates) — shell module",
		MultiInstance: module.MultiInstanceAll,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.AppTemplate{},
		&model.AppDeployment{},
		&model.ChartRepository{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "appstore", Actions: []string{"view", "create", "edit", "delete"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	// HTTP APIs not implemented — hide from navigation until ready.
	return nil
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	// No HTTP routes yet.
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	if host != nil {
		m.db = host.DB
	}
	return nil
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.db == nil {
		return map[string]any{
			"api_ready": false,
			"note":      "shell module — HTTP APIs not implemented",
		}
	}
	var templates, enabledTemplates, deployments, repos, enabledRepos int64
	_ = m.db.Model(&model.AppTemplate{}).Count(&templates).Error
	_ = m.db.Model(&model.AppTemplate{}).Where("enabled = ?", true).Count(&enabledTemplates).Error
	_ = m.db.Model(&model.AppDeployment{}).Count(&deployments).Error
	_ = m.db.Model(&model.ChartRepository{}).Count(&repos).Error
	_ = m.db.Model(&model.ChartRepository{}).Where("enabled = ?", true).Count(&enabledRepos).Error
	return map[string]any{
		"api_ready":          false,
		"templates":          templates,
		"enabled_templates":  enabledTemplates,
		"deployments":        deployments,
		"chart_repos":        repos,
		"enabled_chart_repos": enabledRepos,
		"note":               "shell module — HTTP APIs not implemented",
	}
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	if m.db == nil && ctx != nil && ctx.Host != nil {
		m.db = ctx.Host.DB
	}
	// Placeholder for future Helm appstore APIs.
}
