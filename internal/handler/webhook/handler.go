package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

// Handler Webhook 处理器
type Handler struct {
	db         *gorm.DB
	httpClient *http.Client
}

// NewHandler 创建 Webhook 处理器
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ListWebhooks 获取 Webhook 列表
func (h *Handler) ListWebhooks(c *gin.Context) {
	var webhooks []model.WebhookConfig
	if err := h.db.Order("created_at DESC").Find(&webhooks).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, webhooks)
}

// CreateWebhook 创建 Webhook
func (h *Handler) CreateWebhook(c *gin.Context) {
	var req struct {
		Name       string   `json:"name" binding:"required"`
		Type       string   `json:"type" binding:"required"`
		URL        string   `json:"url" binding:"required"`
		Secret     string   `json:"secret"`
		Events     []string `json:"events"`
		Namespaces []string `json:"namespaces"`
		Severity   string   `json:"severity"`
		Template   string   `json:"template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	eventsJSON, _ := json.Marshal(req.Events)
	namespacesJSON, _ := json.Marshal(req.Namespaces)

	webhook := model.WebhookConfig{
		Name:       req.Name,
		Type:       req.Type,
		URL:        req.URL,
		Secret:     req.Secret,
		Events:     string(eventsJSON),
		Namespaces: string(namespacesJSON),
		Severity:   req.Severity,
		Template:   req.Template,
		Enabled:    true,
	}

	if err := h.db.Create(&webhook).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, webhook)
}

// UpdateWebhook 更新 Webhook
func (h *Handler) UpdateWebhook(c *gin.Context) {
	id := c.Param("id")
	var webhook model.WebhookConfig
	if err := h.db.First(&webhook, id).Error; err != nil {
		response.NotFound(c, "webhook not found")
		return
	}

	var req struct {
		Name       string   `json:"name"`
		URL        string   `json:"url"`
		Secret     string   `json:"secret"`
		Events     []string `json:"events"`
		Namespaces []string `json:"namespaces"`
		Severity   string   `json:"severity"`
		Template   string   `json:"template"`
		Enabled    *bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if req.Name != "" {
		webhook.Name = req.Name
	}
	if req.URL != "" {
		webhook.URL = req.URL
	}
	if req.Secret != "" {
		webhook.Secret = req.Secret
	}
	if req.Events != nil {
		eventsJSON, _ := json.Marshal(req.Events)
		webhook.Events = string(eventsJSON)
	}
	if req.Namespaces != nil {
		namespacesJSON, _ := json.Marshal(req.Namespaces)
		webhook.Namespaces = string(namespacesJSON)
	}
	if req.Severity != "" {
		webhook.Severity = req.Severity
	}
	if req.Template != "" {
		webhook.Template = req.Template
	}
	if req.Enabled != nil {
		webhook.Enabled = *req.Enabled
	}

	if err := h.db.Save(&webhook).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, webhook)
}

// DeleteWebhook 删除 Webhook
func (h *Handler) DeleteWebhook(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&model.WebhookConfig{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.SuccessWithMessage(c, "webhook deleted", nil)
}

// TestWebhook 测试 Webhook
func (h *Handler) TestWebhook(c *gin.Context) {
	id := c.Param("id")
	var webhook model.WebhookConfig
	if err := h.db.First(&webhook, id).Error; err != nil {
		response.NotFound(c, "webhook not found")
		return
	}

	// 发送测试消息
	testPayload := map[string]interface{}{
		"type":      "test",
		"message":   "KubePilot Webhook 测试消息",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	log := h.sendWebhook(&webhook, "test", testPayload)

	if log.Status == "success" {
		response.Success(c, gin.H{"message": "测试成功"})
	} else {
		response.BadRequest(c, fmt.Sprintf("测试失败: %s", log.Error))
	}
}

// ListWebhookLogs 获取 Webhook 日志
func (h *Handler) ListWebhookLogs(c *gin.Context) {
	var logs []model.WebhookLog
	query := h.db.Preload("Webhook").Order("created_at DESC").Limit(100)

	if webhookID := c.Query("webhook_id"); webhookID != "" {
		query = query.Where("webhook_id = ?", webhookID)
	}

	if err := query.Find(&logs).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, logs)
}

// SendNotification 发送通知（供其他模块调用）
func (h *Handler) SendNotification(eventType string, data map[string]interface{}) {
	var webhooks []model.WebhookConfig
	h.db.Where("enabled = ?", true).Find(&webhooks)

	for _, webhook := range webhooks {
		// 检查事件类型是否匹配
		if !h.matchEvent(&webhook, eventType) {
			continue
		}

		go h.sendWebhook(&webhook, eventType, data)
	}
}

// matchEvent 检查事件是否匹配
func (h *Handler) matchEvent(webhook *model.WebhookConfig, eventType string) bool {
	if webhook.Events == "" {
		return true // 空表示所有事件
	}

	var events []string
	json.Unmarshal([]byte(webhook.Events), &events)

	for _, e := range events {
		if e == eventType || e == "all" {
			return true
		}
	}
	return false
}

// sendWebhook 发送 Webhook
func (h *Handler) sendWebhook(webhook *model.WebhookConfig, eventType string, data map[string]interface{}) *model.WebhookLog {
	payload := map[string]interface{}{
		"event_type": eventType,
		"data":       data,
		"timestamp":  time.Now().Format(time.RFC3339),
		"source":     "kubepilot",
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(body))
	if err != nil {
		return h.saveLog(webhook, eventType, string(body), webhook.URL, "", 0, err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KubePilot-Webhook/1.0")
	if webhook.Secret != "" {
		req.Header.Set("X-Webhook-Secret", webhook.Secret)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return h.saveLog(webhook, eventType, string(body), webhook.URL, "", 0, err.Error())
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// 更新最后触发时间
	now := time.Now()
	webhook.LastFiredAt = &now
	h.db.Save(webhook)

	return h.saveLog(webhook, eventType, string(body), webhook.URL, string(respBody), resp.StatusCode, "")
}

// saveLog 保存日志
func (h *Handler) saveLog(webhook *model.WebhookConfig, eventType, reqBody, reqURL, respBody string, statusCode int, errMsg string) *model.WebhookLog {
	status := "success"
	if statusCode >= 400 || errMsg != "" {
		status = "failed"
	}

	log := &model.WebhookLog{
		WebhookID:   webhook.ID,
		EventType:   eventType,
		RequestURL:  reqURL,
		RequestBody: reqBody,
		Response:    respBody,
		StatusCode:  statusCode,
		Error:       errMsg,
		Status:      status,
	}
	h.db.Create(log)
	return log
}

