package aiops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/service/aiops"
	"gorm.io/gorm"
)

// aiopsResult aiops包的ExecuteResult类型别名
type aiopsResult = aiops.ExecuteResult

// Handler AIOps处理器
type Handler struct {
	service    *aiops.Service
	db         *gorm.DB
	encryptKey string
}

// NewHandler 创建AIOps处理器
func NewHandler(service *aiops.Service, db *gorm.DB, encryptKey string) *Handler {
	return &Handler{
		service:    service,
		db:         db,
		encryptKey: encryptKey,
	}
}

// Chat 智能对话
func (h *Handler) Chat(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured. Please set LLM API key in config.yaml")
		return
	}

	userID, _ := c.Get("user_id")

	var req aiops.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if req.ClusterID > 0 && !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, "*") {
		return
	}

	result, err := h.service.Chat(c.Request.Context(), userID.(uint), &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// ChatStream 流式对话
func (h *Handler) ChatStream(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured. Please set LLM API key in config.yaml")
		return
	}

	userID, _ := c.Get("user_id")

	var req aiops.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	ch, err := h.service.ChatStream(c.Request.Context(), userID.(uint), &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	c.Writer.Flush()

	// 发送流式数据
	for chunk := range ch {
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}
}

// Diagnose 智能诊断
func (h *Handler) Diagnose(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured. Please set LLM API key in config.yaml")
		return
	}

	var req aiops.DiagnosisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	ns := req.Namespace
	if ns == "" {
		ns = "*"
	}
	if !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, ns) {
		return
	}

	result, err := h.service.Diagnose(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// ClearHistory 清除对话历史
func (h *Handler) ClearHistory(c *gin.Context) {
	userID, _ := c.Get("user_id")
	if h.service != nil {
		h.service.ClearHistory(userID.(uint))
	}
	response.SuccessWithMessage(c, "history cleared", nil)
}

// ChatRequest 聊天请求（用于前端）
type ChatRequest struct {
	Message   string `json:"message"`
	ClusterID uint   `json:"cluster_id"`
	Context   string `json:"context"`
}

// ChatSSE 处理SSE聊天请求
func (h *Handler) ChatSSE(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ClusterID > 0 && !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, "*") {
		return
	}

	aiopsReq := &aiops.ChatRequest{
		Message:   req.Message,
		ClusterID: req.ClusterID,
		Context:   req.Context,
	}

	ch, err := h.service.ChatStream(c.Request.Context(), userID.(uint), aiopsReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置SSE响应
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	for chunk := range ch {
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// DiagnoseRequest 诊断请求（用于前端）
type DiagnoseRequest struct {
	ClusterID    uint   `json:"cluster_id"`
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
	Namespace    string `json:"namespace"`
	Problem      string `json:"problem"`
}

// DiagnoseResource 诊断资源问题
func (h *Handler) DiagnoseResource(c *gin.Context) {
	var req DiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	aiopsReq := &aiops.DiagnosisRequest{
		ClusterID:    req.ClusterID,
		ResourceType: req.ResourceType,
		ResourceName: req.ResourceName,
		Namespace:    req.Namespace,
		Problem:      req.Problem,
	}

	result, err := h.service.Diagnose(c.Request.Context(), aiopsReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// ==================== LLM 配置管理 ====================

// ListLLMConfigs 获取所有LLM配置
func (h *Handler) ListLLMConfigs(c *gin.Context) {
	var configs []model.LLMConfig
	if err := h.db.Order("is_active DESC, id DESC").Find(&configs).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 隐藏API Key
	result := make([]gin.H, 0, len(configs))
	for _, cfg := range configs {
		apiKey, err := crypto.OpenSecret(cfg.APIKey, h.encryptKey)
		if err != nil {
			response.InternalError(c, "failed to decrypt LLM key")
			return
		}
		result = append(result, gin.H{
			"id":                 cfg.ID,
			"provider":           cfg.Provider,
			"api_key":            maskAPIKey(apiKey),
			"base_url":           cfg.BaseURL,
			"model":              cfg.Model,
			"temperature":        cfg.Temperature,
			"max_tokens":         cfg.MaxTokens,
			"timeout":            cfg.Timeout,
			"input_price_per_m":  cfg.InputPricePerM,
			"output_price_per_m": cfg.OutputPricePerM,
			"is_active":          cfg.IsActive,
			"created_at":         cfg.CreatedAt,
		})
	}

	response.Success(c, result)
}

// GetLLMConfig 获取当前默认LLM配置
func (h *Handler) GetLLMConfig(c *gin.Context) {
	var config model.LLMConfig
	result := h.db.Where("is_active = ?", true).Order("id desc").First(&config)
	if result.Error != nil {
		// 返回空配置
		response.Success(c, gin.H{
			"configured": false,
			"provider":   "openai",
			"model":      "gpt-3.5-turbo",
		})
		return
	}

	// 隐藏API Key中间部分
	apiKey, err := crypto.OpenSecret(config.APIKey, h.encryptKey)
	if err != nil {
		response.InternalError(c, "failed to decrypt LLM key")
		return
	}
	maskedKey := maskAPIKey(apiKey)

	response.Success(c, gin.H{
		"configured":         true,
		"id":                 config.ID,
		"provider":           config.Provider,
		"api_key":            maskedKey,
		"base_url":           config.BaseURL,
		"model":              config.Model,
		"temperature":        config.Temperature,
		"max_tokens":         config.MaxTokens,
		"timeout":            config.Timeout,
		"input_price_per_m":  config.InputPricePerM,
		"output_price_per_m": config.OutputPricePerM,
	})
}

// GetLLMConfigByID 获取指定ID的LLM配置
func (h *Handler) GetLLMConfigByID(c *gin.Context) {
	id := c.Param("id")

	var config model.LLMConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "config not found")
		return
	}
	apiKey, err := crypto.OpenSecret(config.APIKey, h.encryptKey)
	if err != nil {
		response.InternalError(c, "failed to decrypt LLM key")
		return
	}

	response.Success(c, gin.H{
		"id":                 config.ID,
		"provider":           config.Provider,
		"api_key":            maskAPIKey(apiKey),
		"base_url":           config.BaseURL,
		"model":              config.Model,
		"temperature":        config.Temperature,
		"max_tokens":         config.MaxTokens,
		"timeout":            config.Timeout,
		"is_active":          config.IsActive,
		"input_price_per_m":  config.InputPricePerM,
		"output_price_per_m": config.OutputPricePerM,
	})
}

// SaveLLMConfig 保存LLM配置
func (h *Handler) SaveLLMConfig(c *gin.Context) {
	var req struct {
		Provider        string  `json:"provider" binding:"required"`
		APIKey          string  `json:"api_key" binding:"required"`
		BaseURL         string  `json:"base_url"`
		Model           string  `json:"model" binding:"required"`
		Temperature     float64 `json:"temperature"`
		MaxTokens       int     `json:"max_tokens"`
		Timeout         int     `json:"timeout"`
		InputPricePerM  float64 `json:"input_price_per_m"`
		OutputPricePerM float64 `json:"output_price_per_m"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 验证 provider
	if req.Provider != "openai" && req.Provider != "anthropic" {
		response.BadRequest(c, "provider must be 'openai' or 'anthropic'")
		return
	}

	// 设置默认值
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 2048
	}
	if req.Timeout == 0 {
		req.Timeout = 120
	}

	// 测试连接
	client, err := llm.NewClient(&llm.LLMConfig{
		Provider:    llm.LLMProvider(req.Provider),
		APIKey:      req.APIKey,
		BaseURL:     req.BaseURL,
		Model:       req.Model,
		Temperature: 0.1,
		MaxTokens:   50,
		Timeout:     30,
	})
	if err != nil {
		response.BadRequest(c, "Failed to create LLM client: "+err.Error())
		return
	}

	testResp, err := client.Chat(c.Request.Context(), &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Say 'OK' in one word."},
		},
	})
	if err != nil {
		response.BadRequest(c, "LLM connection test failed: "+err.Error())
		return
	}

	if testResp.Content == "" {
		response.BadRequest(c, "LLM returned empty response")
		return
	}
	sealedAPIKey, err := crypto.SealSecret(req.APIKey, h.encryptKey)
	if err != nil {
		response.InternalError(c, "failed to encrypt LLM key")
		return
	}

	// 将所有现有配置设为非活跃
	h.db.Model(&model.LLMConfig{}).Where("is_active = ?", true).Update("is_active", false)

	// 创建新配置
	config := model.LLMConfig{
		Provider:        req.Provider,
		APIKey:          sealedAPIKey,
		BaseURL:         req.BaseURL,
		Model:           req.Model,
		Temperature:     req.Temperature,
		MaxTokens:       req.MaxTokens,
		Timeout:         req.Timeout,
		IsActive:        true,
		InputPricePerM:  req.InputPricePerM,
		OutputPricePerM: req.OutputPricePerM,
	}

	if err := h.db.Create(&config).Error; err != nil {
		response.InternalError(c, "failed to save config: "+err.Error())
		return
	}

	// 更新服务配置
	if h.service != nil {
		h.service.UpdateConfig(&llm.LLMConfig{
			Provider:    llm.LLMProvider(req.Provider),
			APIKey:      req.APIKey,
			BaseURL:     req.BaseURL,
			Model:       req.Model,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			Timeout:     req.Timeout,
		})
	}

	response.SuccessWithMessage(c, "LLM config saved successfully", gin.H{
		"id":       config.ID,
		"provider": config.Provider,
		"model":    config.Model,
	})
}

// UpdateLLMConfig 更新LLM配置
func (h *Handler) UpdateLLMConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.LLMConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "config not found")
		return
	}

	var req struct {
		APIKey          string   `json:"api_key"`
		BaseURL         string   `json:"base_url"`
		Model           string   `json:"model"`
		Temperature     float64  `json:"temperature"`
		MaxTokens       int      `json:"max_tokens"`
		Timeout         int      `json:"timeout"`
		InputPricePerM  *float64 `json:"input_price_per_m"`
		OutputPricePerM *float64 `json:"output_price_per_m"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 更新字段
	if req.APIKey != "" {
		sealed, err := crypto.SealSecret(req.APIKey, h.encryptKey)
		if err != nil {
			response.InternalError(c, "failed to encrypt LLM key")
			return
		}
		config.APIKey = sealed
	}
	if req.BaseURL != "" {
		config.BaseURL = req.BaseURL
	}
	if req.Model != "" {
		config.Model = req.Model
	}
	if req.Temperature > 0 {
		config.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		config.MaxTokens = req.MaxTokens
	}
	if req.Timeout > 0 {
		config.Timeout = req.Timeout
	}
	if req.InputPricePerM != nil {
		config.InputPricePerM = *req.InputPricePerM
	}
	if req.OutputPricePerM != nil {
		config.OutputPricePerM = *req.OutputPricePerM
	}

	if err := h.db.Save(&config).Error; err != nil {
		response.InternalError(c, "failed to update config")
		return
	}

	// 如果是当前活跃配置，更新服务
	if config.IsActive && h.service != nil {
		apiKey, err := crypto.OpenSecret(config.APIKey, h.encryptKey)
		if err != nil {
			response.InternalError(c, "failed to decrypt LLM key")
			return
		}
		h.service.UpdateConfig(&llm.LLMConfig{
			Provider:    llm.LLMProvider(config.Provider),
			APIKey:      apiKey,
			BaseURL:     config.BaseURL,
			Model:       config.Model,
			Temperature: config.Temperature,
			MaxTokens:   config.MaxTokens,
			Timeout:     config.Timeout,
		})
	}

	response.SuccessWithMessage(c, "config updated", nil)
}

// DeleteLLMConfig 删除LLM配置
func (h *Handler) DeleteLLMConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.LLMConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "config not found")
		return
	}

	var replacement model.LLMConfig
	if config.IsActive {
		// Keep an active runtime configuration at all times. The replacement is
		// selected before deleting, then promoted atomically with the deletion.
		if err := h.db.Where("id <> ?", config.ID).Order("id DESC").First(&replacement).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				response.BadRequest(c, "cannot delete the only LLM config; create another config first")
				return
			}
			response.InternalError(c, "failed to select replacement config")
			return
		}
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if config.IsActive {
			if err := tx.Model(&model.LLMConfig{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&replacement).Update("is_active", true).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&config).Error
	}); err != nil {
		response.InternalError(c, "failed to delete config")
		return
	}

	if config.IsActive && h.service != nil {
		apiKey, err := crypto.OpenSecret(replacement.APIKey, h.encryptKey)
		if err != nil {
			response.InternalError(c, "config deleted but replacement LLM key is unavailable")
			return
		}
		if err := h.service.UpdateConfig(&llm.LLMConfig{
			Provider:    llm.LLMProvider(replacement.Provider),
			APIKey:      apiKey,
			BaseURL:     replacement.BaseURL,
			Model:       replacement.Model,
			Temperature: replacement.Temperature,
			MaxTokens:   replacement.MaxTokens,
			Timeout:     replacement.Timeout,
		}); err != nil {
			response.InternalError(c, "config deleted but failed to activate replacement: "+err.Error())
			return
		}
	}

	response.SuccessWithMessage(c, "config deleted", nil)
}

// SetDefaultLLMConfig 设置默认LLM配置
func (h *Handler) SetDefaultLLMConfig(c *gin.Context) {
	id := c.Param("id")

	var config model.LLMConfig
	if err := h.db.First(&config, id).Error; err != nil {
		response.NotFound(c, "config not found")
		return
	}

	// 将所有配置设为非活跃
	h.db.Model(&model.LLMConfig{}).Where("is_active = ?", true).Update("is_active", false)

	// 设置当前配置为活跃
	config.IsActive = true
	if err := h.db.Save(&config).Error; err != nil {
		response.InternalError(c, "failed to set default config")
		return
	}

	// 更新服务配置
	if h.service != nil {
		apiKey, err := crypto.OpenSecret(config.APIKey, h.encryptKey)
		if err != nil {
			response.InternalError(c, "failed to decrypt LLM key")
			return
		}
		h.service.UpdateConfig(&llm.LLMConfig{
			Provider:    llm.LLMProvider(config.Provider),
			APIKey:      apiKey,
			BaseURL:     config.BaseURL,
			Model:       config.Model,
			Temperature: config.Temperature,
			MaxTokens:   config.MaxTokens,
			Timeout:     config.Timeout,
		})
	}

	response.SuccessWithMessage(c, "default config set", nil)
}

// TestLLMConfig 测试LLM连接。
// 编辑已有配置时允许 api_key 留空：传入 id 后复用数据库中的密钥。
func (h *Handler) TestLLMConfig(c *gin.Context) {
	var req struct {
		ID       uint   `json:"id"`
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		BaseURL  string `json:"base_url"`
		Model    string `json:"model"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.ID > 0 {
		var stored model.LLMConfig
		if err := h.db.First(&stored, req.ID).Error; err != nil {
			response.NotFound(c, "config not found")
			return
		}
		if req.Provider == "" {
			req.Provider = stored.Provider
		}
		if req.APIKey == "" || strings.Contains(req.APIKey, "****") {
			apiKey, err := crypto.OpenSecret(stored.APIKey, h.encryptKey)
			if err != nil {
				response.InternalError(c, "failed to decrypt LLM key")
				return
			}
			req.APIKey = apiKey
		}
		if req.BaseURL == "" {
			req.BaseURL = stored.BaseURL
		}
		if req.Model == "" {
			req.Model = stored.Model
		}
	}

	if req.Provider == "" || req.Model == "" {
		response.BadRequest(c, "provider and model are required")
		return
	}
	if req.APIKey == "" {
		response.BadRequest(c, "api_key is required (leave blank only when testing an existing config by id)")
		return
	}

	client, err := llm.NewClient(&llm.LLMConfig{
		Provider:    llm.LLMProvider(req.Provider),
		APIKey:      req.APIKey,
		BaseURL:     req.BaseURL,
		Model:       req.Model,
		Temperature: 0.1,
		MaxTokens:   50,
		Timeout:     30,
	})
	if err != nil {
		response.BadRequest(c, "Failed to create client: "+err.Error())
		return
	}

	// 发送测试消息
	resp, err := client.Chat(c.Request.Context(), &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Say 'OK' in one word."},
		},
	})
	if err != nil {
		response.BadRequest(c, "Connection failed: "+err.Error())
		return
	}

	if resp.Content == "" {
		response.BadRequest(c, "Empty response from API")
		return
	}

	response.Success(c, gin.H{
		"success": true,
		"message": "Connection successful",
		"model":   req.Model,
	})
}

// ==================== AI Agent ====================

// AgentChat Agent对话 - 自然语言操作K8S
func (h *Handler) AgentChat(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	userID, _ := c.Get("user_id")

	var req struct {
		Message        string `json:"message" binding:"required"`
		ClusterID      uint   `json:"cluster_id" binding:"required"`
		ConversationID uint   `json:"conversation_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, "*") {
		return
	}
	if !h.validateAgentConversation(c, req.ConversationID, req.ClusterID) {
		return
	}

	result, err := h.service.AgentChat(c.Request.Context(), userID.(uint), req.ClusterID, req.Message, req.ConversationID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// AgentChatStream Agent 对话 SSE：工具过程 + 最终回答增量
func (h *Handler) AgentChatStream(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	userID, _ := c.Get("user_id")

	var req struct {
		Message        string `json:"message" binding:"required"`
		ClusterID      uint   `json:"cluster_id" binding:"required"`
		ConversationID uint   `json:"conversation_id"`
		Retry          bool   `json:"retry"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, "*") {
		return
	}
	if !h.validateAgentConversation(c, req.ConversationID, req.ClusterID) {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	_ = h.service.AgentChatStream(c.Request.Context(), userID.(uint), req.ClusterID, req.ConversationID, req.Message, req.Retry, func(ev aiops.AgentStreamEvent) {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	})
}

// AgentRetryReadTool repeats one failed query without running an Agent turn.
func (h *Handler) AgentRetryReadTool(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}
	var req struct {
		ClusterID      uint   `json:"cluster_id" binding:"required"`
		ConversationID uint   `json:"conversation_id" binding:"required"`
		Name           string `json:"name" binding:"required"`
		Args           string `json:"args" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, "*") || !h.validateAgentConversation(c, req.ConversationID, req.ClusterID) {
		return
	}
	userID, _ := c.Get("user_id")
	item, err := h.service.RetryAgentQuery(c.Request.Context(), userID.(uint), req.ClusterID, req.ConversationID, req.Name, req.Args)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, item)
}

// AgentListPending 列出会话下待确认写操作
func (h *Handler) AgentListPending(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}
	userID, _ := c.Get("user_id")
	var convID uint
	if _, err := fmt.Sscanf(c.Query("conversation_id"), "%d", &convID); err != nil || convID == 0 {
		response.BadRequest(c, "conversation_id is required")
		return
	}
	list, err := h.service.ListPendingActions(userID.(uint), convID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"pending_actions": list})
}

// AgentCancelPending 取消会话下待确认写操作
func (h *Handler) AgentCancelPending(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}
	userID, _ := c.Get("user_id")
	var req struct {
		ConversationID uint   `json:"conversation_id" binding:"required"`
		ActionIDs      []uint `json:"action_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if err := h.service.CancelPendingActions(userID.(uint), req.ConversationID, req.ActionIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.SuccessWithMessage(c, "cancelled", nil)
}

// AgentConfirmAction 确认执行已暂存的 Agent 动作（必须先 dry-run / stage）。
func (h *Handler) AgentConfirmAction(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	actionID := c.Param("actionId")
	var action model.AgentAction
	if err := h.db.First(&action, actionID).Error; err != nil {
		response.NotFound(c, "action not found")
		return
	}
	userID, _ := c.Get("user_id")
	roleID, _ := c.Get("role_id")
	production, err := h.productionAction(action.ClusterID)
	if err != nil {
		response.BadRequest(c, "cluster unavailable")
		return
	}
	if production && action.UserID != userID.(uint) {
		response.Forbidden(c, "only the requester can submit or execute this production change")
		return
	}
	if action.UserID != 0 && action.UserID != userID.(uint) {
		var role model.Role
		if err := h.db.First(&role, roleID).Error; err != nil || !(role.IsSystem || role.Name == "admin") {
			response.Forbidden(c, "cannot confirm another user's action")
			return
		}
	}

	ns := action.Namespace
	if ns == "" {
		ns = "*"
	}
	if !authz.EnsureScope(c, "aiops", "execute", action.ClusterID, ns) {
		return
	}
	if production && action.Status == "pending" {
		if err := h.submitActionForApproval(action, userID.(uint)); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.Success(c, gin.H{"success": true, "action_id": action.ID, "status": "approval_pending", "message": "已提交，等待另一名有权限的人员审批"})
		return
	}
	expectedStatus := "pending"
	if production {
		expectedStatus = "approved"
		if action.ApprovedBy == nil || *action.ApprovedBy == action.UserID {
			response.Forbidden(c, "independent approval required")
			return
		}
	}
	if action.Status != expectedStatus {
		response.BadRequest(c, "action is not ready for execution")
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&model.AgentAction{}).Where("id = ? AND status = ?", action.ID, expectedStatus).Update("status", "executing")
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return fmt.Errorf("action was already claimed")
		}
		return tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: userID.(uint), Event: "executing", Detail: "confirmed staged change"}).Error
	})
	if err != nil {
		response.BadRequest(c, "action was already claimed")
		return
	}

	var result *aiopsResult
	var observation, rollback string
	if production {
		result, observation, rollback, err = h.service.ExecuteObservedDeploymentChange(c.Request.Context(), &action)
	} else {
		result, err = h.service.ExecuteStagedActionWithRetry(c.Request.Context(), &action)
	}
	action.Observation = observation
	action.RollbackResult = rollback
	if err != nil {
		action.Status = "failed"
		if strings.HasPrefix(rollback, "rollback verified") {
			action.Status = "rolled_back"
		}
		if strings.HasPrefix(rollback, "rollback submitted") {
			action.Status = "rollback_pending"
		}
		action.Result = err.Error()
		if saveErr := h.recordActionOutcome(&action, userID.(uint), "failed", err.Error()); saveErr != nil {
			response.InternalError(c, "change result could not be audited")
			return
		}
		response.InternalError(c, err.Error()+"; "+rollback)
		return
	}
	if result == nil || !result.Success {
		msg := "execution failed"
		if result != nil {
			msg = result.Message
		}
		action.Status = "failed"
		action.Result = msg
		if saveErr := h.recordActionOutcome(&action, userID.(uint), "failed", msg); saveErr != nil {
			response.InternalError(c, "change result could not be audited")
			return
		}
		response.BadRequest(c, msg)
		return
	}

	action.Status = "executed"
	action.Result = result.Message
	now := time.Now()
	action.ExecutedAt = &now
	if err := h.recordActionOutcome(&action, userID.(uint), "executed", result.Message); err != nil {
		response.InternalError(c, "change result could not be audited")
		return
	}

	response.Success(c, gin.H{
		"success":   true,
		"message":   result.Message,
		"details":   result.Details,
		"action_id": action.ID,
		"status":    action.Status,
	})
}

// AgentExecute 仅暂存写操作并返回 dry-run 结果，不会直接改集群。
// 真正执行必须调用 AgentConfirmAction。
func (h *Handler) AgentExecute(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		ClusterID      uint                  `json:"cluster_id" binding:"required"`
		Action         string                `json:"action" binding:"required"`
		Namespace      string                `json:"namespace"`
		Name           string                `json:"name"`
		Image          string                `json:"image"`
		Replicas       int32                 `json:"replicas"`
		Ports          []int32               `json:"ports"`
		ServiceType    string                `json:"service_type"`
		Port           int32                 `json:"port"`
		TargetPort     int32                 `json:"target_port"`
		NodePort       int32                 `json:"node_port"`
		Selector       map[string]string     `json:"selector"`
		HostPathMounts []aiops.HostPathMount `json:"host_path_mounts"`
		ConversationID uint                  `json:"conversation_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if req.Name == "" {
		response.BadRequest(c, "resource name is required")
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, req.Namespace) {
		return
	}
	if req.Image == "" {
		req.Image = "nginx:latest"
	}
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if req.ServiceType == "" {
		req.ServiceType = "ClusterIP"
	}
	if req.Port == 0 {
		req.Port = 80
	}
	if req.TargetPort == 0 {
		req.TargetPort = 80
	}

	params := aiops.StagedActionParams{
		Action:         req.Action,
		Namespace:      req.Namespace,
		Name:           req.Name,
		Image:          req.Image,
		Replicas:       req.Replicas,
		Ports:          req.Ports,
		ServiceType:    req.ServiceType,
		Port:           req.Port,
		TargetPort:     req.TargetPort,
		NodePort:       req.NodePort,
		Selector:       req.Selector,
		HostPathMounts: req.HostPathMounts,
	}
	dryRun, err := h.service.DryRunStagedAction(c.Request.Context(), req.ClusterID, params)
	if err != nil {
		response.BadRequest(c, "dry-run failed: "+err.Error())
		return
	}
	resourceUID, baseGeneration, err := h.service.DeploymentPrecondition(c.Request.Context(), req.ClusterID, params)
	if err != nil {
		response.BadRequest(c, "resource snapshot failed: "+err.Error())
		return
	}

	paramBytes, _ := json.Marshal(params)
	userID, _ := c.Get("user_id")
	actionType, resourceType := stagedActionMeta(req.Action)
	action := model.AgentAction{
		UserID:         userID.(uint),
		ActionType:     actionType,
		ResourceType:   resourceType,
		ResourceName:   req.Name,
		Namespace:      req.Namespace,
		ClusterID:      req.ClusterID,
		Description:    fmt.Sprintf("%s %s/%s", req.Action, req.Namespace, req.Name),
		Parameters:     string(paramBytes),
		DryRunResult:   dryRun,
		ResourceUID:    resourceUID,
		BaseGeneration: baseGeneration,
		Status:         "pending",
	}
	if req.ConversationID > 0 {
		cid := req.ConversationID
		action.ConversationID = &cid
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&action).Error; err != nil {
			return err
		}
		return tx.Create(&model.AgentActionAudit{ActionID: action.ID, ActorID: userID.(uint), Event: "staged", Detail: "server dry-run preview created"}).Error
	}); err != nil {
		response.InternalError(c, "failed to stage action: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"success":      true,
		"staged":       true,
		"action_id":    action.ID,
		"status":       action.Status,
		"dry_run":      dryRun,
		"message":      "action staged; confirm required before execution",
		"confirm_path": fmt.Sprintf("/api/v1/aiops/agent/confirm/%d", action.ID),
	})
}

func stagedActionMeta(action string) (actionType, resourceType string) {
	switch action {
	case "create_deployment":
		return "create", "deployments"
	case "create_service":
		return "create", "services"
	case "delete_deployment":
		return "delete", "deployments"
	case "delete_service":
		return "delete", "services"
	case "delete_pod":
		return "delete", "pods"
	case "scale_deployment":
		return "scale", "deployments"
	default:
		return "execute", "unknown"
	}
}

// KubectlExecute 执行kubectl命令
func (h *Handler) KubectlExecute(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		ClusterID uint     `json:"cluster_id" binding:"required"`
		Command   string   `json:"command" binding:"required"`
		Args      []string `json:"args"`
		YAML      string   `json:"yaml"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if !authz.EnsureScope(c, "aiops", "execute", req.ClusterID, "*") {
		return
	}
	// All writes must pass through staged preview and the production approval gate.
	switch req.Command {
	case "get", "describe", "logs", "top", "explain", "api-resources", "version":
	default:
		response.BadRequest(c, "kubectl write commands are disabled here; use staged Agent changes")
		return
	}

	ctx := c.Request.Context()

	// 构建参数
	args := append([]string{req.Command}, req.Args...)
	result, err := h.service.ExecuteKubectl(ctx, req.ClusterID, args)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// KubectlQuery 查询K8S资源
func (h *Handler) KubectlQuery(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	if !authz.EnsureScope(c, "aiops", "view", uint(clusterID), "*") {
		return
	}

	queryType := c.Query("type")
	if queryType == "" {
		queryType = "all"
	}

	ctx := c.Request.Context()
	result, err := h.service.QueryWithKubectl(ctx, uint(clusterID), queryType)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// maskAPIKey 隐藏API Key中间部分
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// ==================== AI 驱动功能 ====================

// ExplainText 划词解释
func (h *Handler) ExplainText(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		Text      string `json:"text" binding:"required"`
		ClusterID uint   `json:"cluster_id"`
		Context   string `json:"context"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.service.ExplainText(c.Request.Context(), &aiops.ExplainRequest{
		Text:      req.Text,
		ClusterID: req.ClusterID,
		Context:   req.Context,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// ExplainTextStream 流式划词解释
func (h *Handler) ExplainTextStream(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		Text      string `json:"text" binding:"required"`
		ClusterID uint   `json:"cluster_id"`
		Context   string `json:"context"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	ch, err := h.service.ExplainTextStream(c.Request.Context(), &aiops.ExplainRequest{
		Text:      req.Text,
		ClusterID: req.ClusterID,
		Context:   req.Context,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "streaming not supported")
		return
	}

	for chunk := range ch {
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// GetResourceGuide 资源指南
func (h *Handler) GetResourceGuide(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		ClusterID    uint   `json:"cluster_id" binding:"required"`
		ResourceType string `json:"resource_type" binding:"required"`
		ResourceName string `json:"resource_name"`
		Namespace    string `json:"namespace"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.service.GetResourceGuide(c.Request.Context(), &aiops.ResourceGuideRequest{
		ClusterID:    req.ClusterID,
		ResourceType: req.ResourceType,
		ResourceName: req.ResourceName,
		Namespace:    req.Namespace,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// TranslateYAML YAML 翻译
func (h *Handler) TranslateYAML(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		YAML      string `json:"yaml" binding:"required"`
		Direction string `json:"direction"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.service.TranslateYAML(c.Request.Context(), &aiops.TranslateYAMLRequest{
		YAML:      req.YAML,
		Direction: req.Direction,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// AnalyzeDescribe Describe 解读
func (h *Handler) AnalyzeDescribe(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		ClusterID    uint   `json:"cluster_id"`
		ResourceType string `json:"resource_type" binding:"required"`
		ResourceName string `json:"resource_name" binding:"required"`
		Namespace    string `json:"namespace"`
		Describe     string `json:"describe"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.service.AnalyzeDescribe(c.Request.Context(), &aiops.AnalyzeDescribeRequest{
		ClusterID:    req.ClusterID,
		ResourceType: req.ResourceType,
		ResourceName: req.ResourceName,
		Namespace:    req.Namespace,
		Describe:     req.Describe,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// AnalyzeLogs 日志问诊
func (h *Handler) AnalyzeLogs(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		ClusterID    uint   `json:"cluster_id" binding:"required"`
		ResourceType string `json:"resource_type"`
		ResourceName string `json:"resource_name" binding:"required"`
		Namespace    string `json:"namespace" binding:"required"`
		Container    string `json:"container"`
		Lines        int    `json:"lines"`
		Logs         string `json:"logs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.service.AnalyzeLogs(c.Request.Context(), &aiops.AnalyzeLogsRequest{
		ClusterID:    req.ClusterID,
		ResourceType: req.ResourceType,
		ResourceName: req.ResourceName,
		Namespace:    req.Namespace,
		Container:    req.Container,
		Lines:        req.Lines,
		Logs:         req.Logs,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// GetTokenUsageStats returns aggregated token usage statistics.
func (h *Handler) GetTokenUsageStats(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "30")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 30
	}
	stats, err := h.service.GetTokenUsageStats(days)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}

// GetTokenUsageRecent returns recent token usage log entries.
func (h *Handler) GetTokenUsageRecent(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 50
	}
	logs, err := h.service.GetTokenUsageRecent(limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, logs)
}
