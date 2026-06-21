package aiops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/service/aiops"
	"gorm.io/gorm"
)

// aiopsResult aiops包的ExecuteResult类型别名
type aiopsResult = aiops.ExecuteResult

// Handler AIOps处理器
type Handler struct {
	service *aiops.Service
	db      *gorm.DB
}

// NewHandler 创建AIOps处理器
func NewHandler(service *aiops.Service, db *gorm.DB) *Handler {
	return &Handler{
		service: service,
		db:      db,
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
	c.Header("Access-Control-Allow-Origin", "*")

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
		result = append(result, gin.H{
			"id":          cfg.ID,
			"provider":    cfg.Provider,
			"api_key":     maskAPIKey(cfg.APIKey),
			"base_url":    cfg.BaseURL,
			"model":       cfg.Model,
			"temperature": cfg.Temperature,
			"max_tokens":  cfg.MaxTokens,
			"timeout":     cfg.Timeout,
			"is_active":   cfg.IsActive,
			"created_at":  cfg.CreatedAt,
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
	maskedKey := maskAPIKey(config.APIKey)

	response.Success(c, gin.H{
		"configured":  true,
		"id":          config.ID,
		"provider":    config.Provider,
		"api_key":     maskedKey,
		"base_url":    config.BaseURL,
		"model":       config.Model,
		"temperature": config.Temperature,
		"max_tokens":  config.MaxTokens,
		"timeout":     config.Timeout,
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

	response.Success(c, gin.H{
		"id":          config.ID,
		"provider":    config.Provider,
		"api_key":     maskAPIKey(config.APIKey),
		"base_url":    config.BaseURL,
		"model":       config.Model,
		"temperature": config.Temperature,
		"max_tokens":  config.MaxTokens,
		"timeout":     config.Timeout,
		"is_active":   config.IsActive,
	})
}

// SaveLLMConfig 保存LLM配置
func (h *Handler) SaveLLMConfig(c *gin.Context) {
	var req struct {
		Provider    string  `json:"provider" binding:"required"`
		APIKey      string  `json:"api_key" binding:"required"`
		BaseURL     string  `json:"base_url"`
		Model       string  `json:"model" binding:"required"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		Timeout     int     `json:"timeout"`
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

	// 将所有现有配置设为非活跃
	h.db.Model(&model.LLMConfig{}).Where("is_active = ?", true).Update("is_active", false)

	// 创建新配置
	config := model.LLMConfig{
		Provider:    req.Provider,
		APIKey:      req.APIKey,
		BaseURL:     req.BaseURL,
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Timeout:     req.Timeout,
		IsActive:    true,
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
		APIKey      string  `json:"api_key"`
		BaseURL     string  `json:"base_url"`
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		Timeout     int     `json:"timeout"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 更新字段
	if req.APIKey != "" {
		config.APIKey = req.APIKey
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

	if err := h.db.Save(&config).Error; err != nil {
		response.InternalError(c, "failed to update config")
		return
	}

	// 如果是当前活跃配置，更新服务
	if config.IsActive && h.service != nil {
		h.service.UpdateConfig(&llm.LLMConfig{
			Provider:    llm.LLMProvider(config.Provider),
			APIKey:      config.APIKey,
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

	if config.IsActive {
		response.BadRequest(c, "cannot delete active config. Set another config as default first")
		return
	}

	if err := h.db.Delete(&config).Error; err != nil {
		response.InternalError(c, "failed to delete config")
		return
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
		h.service.UpdateConfig(&llm.LLMConfig{
			Provider:    llm.LLMProvider(config.Provider),
			APIKey:      config.APIKey,
			BaseURL:     config.BaseURL,
			Model:       config.Model,
			Temperature: config.Temperature,
			MaxTokens:   config.MaxTokens,
			Timeout:     config.Timeout,
		})
	}

	response.SuccessWithMessage(c, "default config set", nil)
}

// TestLLMConfig 测试LLM连接
func (h *Handler) TestLLMConfig(c *gin.Context) {
	var req struct {
		Provider string `json:"provider" binding:"required"`
		APIKey   string `json:"api_key" binding:"required"`
		BaseURL  string `json:"base_url"`
		Model    string `json:"model" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
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

	result, err := h.service.AgentChat(c.Request.Context(), userID.(uint), req.ClusterID, req.Message, req.ConversationID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// AgentConfirmAction 确认执行Agent动作
func (h *Handler) AgentConfirmAction(c *gin.Context) {
	actionID := c.Param("actionId")

	var action model.AgentAction
	if err := h.db.First(&action, actionID).Error; err != nil {
		response.NotFound(c, "action not found")
		return
	}

	if action.Status != "pending" {
		response.BadRequest(c, "action is not pending")
		return
	}

	// 执行动作
	result, err := h.service.ExecuteAgentAction(c.Request.Context(), &action)
	if err != nil {
		action.Status = "failed"
		action.Result = err.Error()
		h.db.Save(&action)
		response.InternalError(c, err.Error())
		return
	}

	action.Status = "executed"
	action.Result = result
	now := time.Now()
	action.ExecutedAt = &now
	h.db.Save(&action)

	response.SuccessWithMessage(c, "action executed", gin.H{
		"result": result,
	})
}

// AgentExecute 执行K8S操作
func (h *Handler) AgentExecute(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}

	var req struct {
		ClusterID   uint              `json:"cluster_id" binding:"required"`
		Action      string            `json:"action" binding:"required"`
		Namespace   string            `json:"namespace"`
		Name        string            `json:"name"`
		Image       string            `json:"image"`
		Replicas    int32             `json:"replicas"`
		Ports       []int32           `json:"ports"`
		ServiceType string            `json:"service_type"`
		Port        int32             `json:"port"`
		TargetPort  int32             `json:"target_port"`
		NodePort    int32             `json:"node_port"`
		Selector    map[string]string `json:"selector"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 验证资源名称
	if req.Name == "" {
		response.BadRequest(c, "resource name is required")
		return
	}

	// 默认命名空间
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	var result *aiopsResult
	var execErr error

	ctx := c.Request.Context()

	switch req.Action {
	case "create_deployment":
		result, execErr = h.service.ExecuteCreateDeployment(ctx, req.ClusterID, req.Namespace, req.Name, req.Image, req.Replicas, req.Ports)
	case "create_service":
		result, execErr = h.service.ExecuteCreateService(ctx, req.ClusterID, req.Namespace, req.Name, req.ServiceType, req.Selector, req.Port, req.TargetPort, req.NodePort)
	case "delete_deployment":
		result, execErr = h.service.ExecuteDeleteDeployment(ctx, req.ClusterID, req.Namespace, req.Name)
	case "delete_service":
		result, execErr = h.service.ExecuteDeleteService(ctx, req.ClusterID, req.Namespace, req.Name)
	case "delete_pod":
		result, execErr = h.service.ExecuteDeletePod(ctx, req.ClusterID, req.Namespace, req.Name)
	case "scale_deployment":
		result, execErr = h.service.ExecuteScaleDeployment(ctx, req.ClusterID, req.Namespace, req.Name, req.Replicas)
	default:
		response.BadRequest(c, "unsupported action: "+req.Action)
		return
	}

	if execErr != nil {
		response.InternalError(c, execErr.Error())
		return
	}

	if result.Success {
		response.Success(c, result)
	} else {
		response.BadRequest(c, result.Message)
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

	ctx := c.Request.Context()

	// 如果是apply命令且有YAML内容
	if req.Command == "apply" && req.YAML != "" {
		result, err := h.service.ExecuteKubectlApply(ctx, req.ClusterID, req.YAML)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		response.Success(c, result)
		return
	}

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
