package aiops

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	aiopsHandler "github.com/kubepilot/kubepilot/internal/handler/aiops"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	aiopsService "github.com/kubepilot/kubepilot/internal/service/aiops"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Module struct {
	module.Base
	handler *aiopsHandler.Handler
	db      *gorm.DB
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Metadata {
	return module.Metadata{
		Name:          "aiops",
		Version:       "1.0.0",
		Description:   "AIOps chat, diagnosis, and kubectl assistant",
		MultiInstance: module.MultiInstanceAll,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.ChatConversation{},
		&model.ChatMessage{},
		&model.AgentAction{},
		&model.AgentToolTrace{},
		&model.LLMConfig{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "aiops", Actions: []string{"view", "create", "edit", "delete", "execute"}},
		{Resource: "aiops_config", Actions: []string{"view", "create", "edit", "delete", "admin", "execute"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	return []module.MenuItem{
		{Key: "/aiops", Label: "AI 智能", Order: 100, Resource: "aiops", Action: "view"},
		{Key: "/aiops/agent", Label: "AI Agent", Parent: "/aiops", Order: 1, Resource: "aiops", Action: "execute"},
		{Key: "/aiops/diagnosis", Label: "智能诊断", Parent: "/aiops", Order: 2, Resource: "aiops", Action: "execute"},
		{Key: "/aiops/tools", Label: "AI 工具箱", Parent: "/aiops", Order: 3, Resource: "aiops", Action: "execute"},
		{Key: "/aiops/settings", Label: "AI 设置", Parent: "/aiops", Order: 4, Resource: "aiops_config", Action: "view"},
	}
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	reg.MustRegister("GET", "/api/v1/aiops/configs", authz.Policy{Resource: "aiops_config", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/configs", authz.Policy{Resource: "aiops_config", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/aiops/configs/default", authz.Policy{Resource: "aiops_config", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/aiops/configs/:id", authz.Policy{Resource: "aiops_config", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("PUT", "/api/v1/aiops/configs/:id", authz.Policy{Resource: "aiops_config", Action: "edit", Scope: authz.ScopePlatform})
	reg.MustRegister("DELETE", "/api/v1/aiops/configs/:id", authz.Policy{Resource: "aiops_config", Action: "delete", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/configs/:id/set-default", authz.Policy{Resource: "aiops_config", Action: "admin", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/configs/test", authz.Policy{Resource: "aiops_config", Action: "execute", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/aiops/conversations", authz.Policy{Resource: "aiops", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/conversations", authz.Policy{Resource: "aiops", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/aiops/conversations/:id", authz.Policy{Resource: "aiops", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("PUT", "/api/v1/aiops/conversations/:id", authz.Policy{Resource: "aiops", Action: "edit", Scope: authz.ScopePlatform})
	reg.MustRegister("DELETE", "/api/v1/aiops/conversations/:id", authz.Policy{Resource: "aiops", Action: "delete", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/conversations/:id/clear", authz.Policy{Resource: "aiops", Action: "edit", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/aiops/conversations/:id/messages", authz.Policy{Resource: "aiops", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/conversations/:id/messages", authz.Policy{Resource: "aiops", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("DELETE", "/api/v1/aiops/conversations/:id/messages/:msgId", authz.Policy{Resource: "aiops", Action: "delete", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/chat", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/chat/stream", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/explain", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/explain/stream", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/resource-guide", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/translate-yaml", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/aiops/analyze-logs", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/diagnose", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/agent", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/agent/stream", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/aiops/agent/pending", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/agent/pending/cancel", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/agent/confirm/:actionId", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/agent/execute", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/aiops/kubectl", authz.Policy{Resource: "aiops", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/aiops/kubectl/:id/query", authz.Policy{Resource: "aiops", Action: "view", Scope: authz.ScopeHandler})
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	if host != nil {
		m.db = host.DB
	}
	if m.db != nil {
		// Staged AI actions may have no conversation; keep conversation_id nullable
		// without a hard FK so dry-run staging can succeed independently.
		for _, constraint := range []string{"fk_agent_actions_conversation"} {
			if m.db.Migrator().HasConstraint(&model.AgentAction{}, constraint) {
				if err := m.db.Migrator().DropConstraint(&model.AgentAction{}, constraint); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.db == nil {
		return nil
	}
	var configs, active, conversations int64
	_ = m.db.Model(&model.LLMConfig{}).Count(&configs).Error
	_ = m.db.Model(&model.LLMConfig{}).Where("is_active = ?", true).Count(&active).Error
	_ = m.db.Model(&model.ChatConversation{}).Count(&conversations).Error
	details := map[string]any{
		"llm_configs":    configs,
		"active_configs": active,
		"conversations":  conversations,
	}
	if configs == 0 {
		details["health_warning"] = "未配置 LLM，AI 功能不可用"
	} else if active == 0 {
		details["health_warning"] = "无激活的 LLM 配置"
	}
	return details
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	if m.db == nil {
		m.db = ctx.Host.DB
	}
	cfg := ctx.Host.Config
	llmConfig := &llm.LLMConfig{
		Provider:    llm.LLMProvider(cfg.LLM.Provider),
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		Model:       cfg.LLM.Model,
		Temperature: cfg.LLM.Temperature,
		MaxTokens:   cfg.LLM.MaxTokens,
		Timeout:     cfg.LLM.Timeout,
	}
	svc, err := aiopsService.NewService(ctx.Host.DB, llmConfig, ctx.Host.EncryptKey, ctx.Host.Cache)
	if err != nil {
		logger.Warn("failed to initialize AIOps service", zap.Error(err))
	}
	m.handler = aiopsHandler.NewHandler(svc, ctx.Host.DB)

	g := protected.Group("/aiops")
	{
		g.GET("/configs", m.handler.ListLLMConfigs)
		g.POST("/configs", m.handler.SaveLLMConfig)
		g.GET("/configs/default", m.handler.GetLLMConfig)
		g.GET("/configs/:id", m.handler.GetLLMConfigByID)
		g.PUT("/configs/:id", m.handler.UpdateLLMConfig)
		g.DELETE("/configs/:id", m.handler.DeleteLLMConfig)
		g.POST("/configs/:id/set-default", m.handler.SetDefaultLLMConfig)
		g.POST("/configs/test", m.handler.TestLLMConfig)

		g.GET("/conversations", m.handler.ListConversations)
		g.POST("/conversations", m.handler.CreateConversation)
		g.GET("/conversations/:id", m.handler.GetConversation)
		g.PUT("/conversations/:id", m.handler.UpdateConversation)
		g.DELETE("/conversations/:id", m.handler.DeleteConversation)
		g.POST("/conversations/:id/clear", m.handler.ClearConversation)

		g.GET("/conversations/:id/messages", m.handler.ListMessages)
		g.POST("/conversations/:id/messages", m.handler.AddMessage)
		g.DELETE("/conversations/:id/messages/:msgId", m.handler.DeleteMessage)

		g.POST("/chat", m.handler.Chat)
		g.POST("/chat/stream", m.handler.ChatStream)
		g.POST("/explain", m.handler.ExplainText)
		g.POST("/explain/stream", m.handler.ExplainTextStream)
		g.POST("/resource-guide", m.handler.GetResourceGuide)
		g.POST("/translate-yaml", m.handler.TranslateYAML)
		g.POST("/analyze-logs", m.handler.AnalyzeLogs)
		g.POST("/diagnose", m.handler.Diagnose)
		g.POST("/agent", m.handler.AgentChat)
		g.POST("/agent/stream", m.handler.AgentChatStream)
		g.GET("/agent/pending", m.handler.AgentListPending)
		g.POST("/agent/pending/cancel", m.handler.AgentCancelPending)
		g.POST("/agent/confirm/:actionId", m.handler.AgentConfirmAction)
		g.POST("/agent/execute", m.handler.AgentExecute)
		g.POST("/kubectl", m.handler.KubectlExecute)
		g.GET("/kubectl/:id/query", m.handler.KubectlQuery)
	}
}
