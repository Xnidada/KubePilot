package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventForwardHandler Event 转发处理器
type EventForwardHandler struct {
	db         *gorm.DB
	httpClient *http.Client
}

func NewEventForwardHandler(db *gorm.DB) *EventForwardHandler {
	return &EventForwardHandler{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ListRules 获取转发规则列表
func (h *EventForwardHandler) ListRules(c *gin.Context) {
	var rules []model.EventForwardRule
	query := h.db.Order("created_at DESC")

	if clusterID := c.Query("cluster_id"); clusterID != "" {
		query = query.Where("cluster_id = ?", clusterID)
	}

	if err := query.Find(&rules).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, rules)
}

// CreateRule 创建转发规则
func (h *EventForwardHandler) CreateRule(c *gin.Context) {
	var rule model.EventForwardRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	rule.Enabled = true
	if err := h.db.Create(&rule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 启动事件监听
	if rule.Enabled {
		go h.startEventWatcher(&rule)
	}

	response.Created(c, rule)
}

// GetRule 获取转发规则详情
func (h *EventForwardHandler) GetRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.EventForwardRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}

	response.Success(c, rule)
}

// UpdateRule 更新转发规则
func (h *EventForwardHandler) UpdateRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.EventForwardRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}

	var req struct {
		Name       string `json:"name"`
		WebhookURL string `json:"webhook_url"`
		Namespaces string `json:"namespaces"`
		Resources  string `json:"resources"`
		EventTypes string `json:"event_types"`
		Reasons    string `json:"reasons"`
		Headers    string `json:"headers"`
		Template   string `json:"template"`
		Enabled    *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.WebhookURL != "" {
		updates["webhook_url"] = req.WebhookURL
	}
	if req.Namespaces != "" {
		updates["namespaces"] = req.Namespaces
	}
	if req.Resources != "" {
		updates["resources"] = req.Resources
	}
	if req.EventTypes != "" {
		updates["event_types"] = req.EventTypes
	}
	if req.Reasons != "" {
		updates["reasons"] = req.Reasons
	}
	if req.Headers != "" {
		updates["headers"] = req.Headers
	}
	if req.Template != "" {
		updates["template"] = req.Template
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.db.Model(&rule).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, rule)
}

// DeleteRule 删除转发规则
func (h *EventForwardHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&model.EventForwardRule{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "rule deleted", nil)
}

// TestRule 测试转发规则
func (h *EventForwardHandler) TestRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.EventForwardRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}

	// 构建测试 payload
	testPayload := map[string]interface{}{
		"type":    "test",
		"message": "KubePilot Event Forward 测试消息",
		"cluster": rule.ClusterID,
		"time":    time.Now().Format(time.RFC3339),
	}

	// 发送测试请求
	err := h.sendWebhook(rule.WebhookURL, rule.Headers, testPayload)

	log := model.EventForwardLog{
		RuleID:    rule.ID,
		ClusterID: rule.ClusterID,
		EventType: "test",
		Message:   "测试消息",
	}

	if err != nil {
		log.Status = "failed"
		log.Error = err.Error()
	} else {
		log.Status = "success"
		log.StatusCode = 200
	}

	h.db.Create(&log)

	if err != nil {
		response.BadRequest(c, fmt.Sprintf("测试失败: %v", err))
		return
	}

	response.Success(c, gin.H{"message": "测试成功"})
}

// ListLogs 获取转发日志
func (h *EventForwardHandler) ListLogs(c *gin.Context) {
	var logs []model.EventForwardLog
	query := h.db.Order("created_at DESC").Limit(100)

	if ruleID := c.Query("rule_id"); ruleID != "" {
		query = query.Where("rule_id = ?", ruleID)
	}

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Find(&logs).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, logs)
}

// startEventWatcher 启动事件监听器
func (h *EventForwardHandler) startEventWatcher(rule *model.EventForwardRule) {
	// 获取集群客户端
	client, err := k8s.Manager.GetClient(rule.ClusterID)
	if err != nil {
		return
	}

	// 解析过滤条件
	var namespaces []string
	var resources []string
	var eventTypes []string
	var reasons []string

	json.Unmarshal([]byte(rule.Namespaces), &namespaces)
	json.Unmarshal([]byte(rule.Resources), &resources)
	json.Unmarshal([]byte(rule.EventTypes), &eventTypes)
	json.Unmarshal([]byte(rule.Reasons), &reasons)

	// 使用 Watch API 监听事件
	watcher, err := client.Clientset.CoreV1().Events("").Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		// 检查规则是否仍然启用
		var currentRule model.EventForwardRule
		if h.db.First(&currentRule, rule.ID).Error != nil || !currentRule.Enabled {
			return
		}

		k8sEvent, ok := event.Object.(*corev1.Event)
		if !ok {
			continue
		}

		// 应用过滤条件
		if !h.matchEvent(k8sEvent, namespaces, resources, eventTypes, reasons) {
			continue
		}

		// 构建 webhook payload
		payload := map[string]interface{}{
			"type":      k8sEvent.Type,
			"reason":    k8sEvent.Reason,
			"message":   k8sEvent.Message,
			"namespace": k8sEvent.Namespace,
			"object":    fmt.Sprintf("%s/%s", k8sEvent.InvolvedObject.Kind, k8sEvent.InvolvedObject.Name),
			"cluster":   rule.ClusterID,
			"first_time": k8sEvent.FirstTimestamp.Time.Format(time.RFC3339),
			"last_time":  k8sEvent.LastTimestamp.Time.Format(time.RFC3339),
			"count":      k8sEvent.Count,
		}

		// 发送 webhook
		err := h.sendWebhook(rule.WebhookURL, rule.Headers, payload)

		// 记录日志
		log := model.EventForwardLog{
			RuleID:    rule.ID,
			ClusterID: rule.ClusterID,
			Namespace: k8sEvent.Namespace,
			Resource:  fmt.Sprintf("%s/%s", k8sEvent.InvolvedObject.Kind, k8sEvent.InvolvedObject.Name),
			EventType: k8sEvent.Type,
			Reason:    k8sEvent.Reason,
			Message:   k8sEvent.Message,
		}

		if err != nil {
			log.Status = "failed"
			log.Error = err.Error()
		} else {
			log.Status = "success"
			log.StatusCode = 200
		}

		h.db.Create(&log)
	}
}

// matchEvent 检查事件是否匹配过滤条件
func (h *EventForwardHandler) matchEvent(event *corev1.Event, namespaces, resources, eventTypes, reasons []string) bool {
	// 检查命名空间
	if len(namespaces) > 0 {
		found := false
		for _, ns := range namespaces {
			if ns == event.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 检查资源类型
	if len(resources) > 0 {
		found := false
		for _, r := range resources {
			if strings.EqualFold(r, event.InvolvedObject.Kind) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 检查事件类型
	if len(eventTypes) > 0 {
		found := false
		for _, t := range eventTypes {
			if t == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 检查 reason
	if len(reasons) > 0 {
		found := false
		for _, r := range reasons {
			if r == event.Reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// sendWebhook 发送 webhook 请求
func (h *EventForwardHandler) sendWebhook(url, headersJSON string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KubePilot-EventForward/1.0")

	// 添加自定义 headers
	if headersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
