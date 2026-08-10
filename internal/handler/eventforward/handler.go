package eventforward

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"go.uber.org/zap"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventForwardHandler Event 转发处理器
type EventForwardHandler struct {
	db         *gorm.DB
	httpClient *http.Client
	logger     *zap.Logger

	mu      sync.Mutex
	cancels map[uint]context.CancelFunc

	eventsSeen    atomic.Int64
	eventsMatched atomic.Int64
	forwardOK     atomic.Int64
	forwardFail   atomic.Int64
}

// StatsSnapshot is a point-in-time view of in-memory forwarding counters.
type StatsSnapshot struct {
	WatchersActive int   `json:"watchers_active"`
	EventsSeen     int64 `json:"events_seen"`
	EventsMatched  int64 `json:"events_matched"`
	ForwardOK      int64 `json:"forward_ok"`
	ForwardFail    int64 `json:"forward_fail"`
}

func NewEventForwardHandler(db *gorm.DB, logger ...*zap.Logger) *EventForwardHandler {
	l := zap.NewNop()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &EventForwardHandler{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:  l,
		cancels: make(map[uint]context.CancelFunc),
	}
}

// StartWatchers resumes watchers for all enabled rules (called on module Start / process restart).
func (h *EventForwardHandler) StartWatchers() error {
	var rules []model.EventForwardRule
	if err := h.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return fmt.Errorf("list event-forward rules: %w", err)
	}
	for i := range rules {
		rule := rules[i]
		h.startEventWatcher(&rule)
	}
	h.logger.Info("event-forward watchers started", zap.Int("count", len(rules)))
	return nil
}

// StopWatchers cancels all active watchers.
func (h *EventForwardHandler) StopWatchers() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, cancel := range h.cancels {
		cancel()
		delete(h.cancels, id)
	}
	h.logger.Info("event-forward watchers stopped")
}

// ActiveWatcherCount reports how many watchers are tracked.
func (h *EventForwardHandler) ActiveWatcherCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.cancels)
}

// Stats returns in-memory watcher and forward counters (process-local, reset on restart).
func (h *EventForwardHandler) Stats() StatsSnapshot {
	return StatsSnapshot{
		WatchersActive: h.ActiveWatcherCount(),
		EventsSeen:     h.eventsSeen.Load(),
		EventsMatched:  h.eventsMatched.Load(),
		ForwardOK:      h.forwardOK.Load(),
		ForwardFail:    h.forwardFail.Load(),
	}
}

// EnabledRuleCount returns how many event-forward rules are enabled in DB.
func (h *EventForwardHandler) EnabledRuleCount() int64 {
	var n int64
	_ = h.db.Model(&model.EventForwardRule{}).Where("enabled = ?", true).Count(&n).Error
	return n
}

// GetStats exposes Stats via HTTP.
func (h *EventForwardHandler) GetStats(c *gin.Context) {
	response.Success(c, h.Stats())
}

// ResetCounters zeroes in-memory forward counters (does not stop watchers).
func (h *EventForwardHandler) ResetCounters() {
	h.eventsSeen.Store(0)
	h.eventsMatched.Store(0)
	h.forwardOK.Store(0)
	h.forwardFail.Store(0)
}

// ResetStats exposes ResetCounters via HTTP and returns the cleared snapshot.
func (h *EventForwardHandler) ResetStats(c *gin.Context) {
	h.ResetCounters()
	response.Success(c, h.Stats())
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
	if !authz.EnsureScope(c, "event_forward", "create", rule.ClusterID, "*") {
		return
	}

	rule.Enabled = true
	if err := h.db.Create(&rule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if rule.Enabled {
		h.startEventWatcher(&rule)
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
	if !authz.EnsureScope(c, "event_forward", "view", rule.ClusterID, "*") {
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
	if !authz.EnsureScope(c, "event_forward", "edit", rule.ClusterID, "*") {
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

	if err := h.db.First(&rule, rule.ID).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	h.stopWatcher(rule.ID)
	if rule.Enabled {
		h.startEventWatcher(&rule)
	}

	response.Success(c, rule)
}

// DeleteRule 删除转发规则
func (h *EventForwardHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	var rule model.EventForwardRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}
	if !authz.EnsureScope(c, "event_forward", "delete", rule.ClusterID, "*") {
		return
	}

	h.stopWatcher(rule.ID)

	if err := h.db.Delete(&rule).Error; err != nil {
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
	if !authz.EnsureScope(c, "event_forward", "execute", rule.ClusterID, "*") {
		return
	}

	testPayload := map[string]interface{}{
		"type":    "test",
		"message": "KubePilot Event Forward 测试消息",
		"cluster": rule.ClusterID,
		"time":    time.Now().Format(time.RFC3339),
	}

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

func (h *EventForwardHandler) stopWatcher(ruleID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.cancels[ruleID]; ok {
		cancel()
		delete(h.cancels, ruleID)
	}
}

// startEventWatcher 启动事件监听器（可取消，支持重启恢复）
func (h *EventForwardHandler) startEventWatcher(rule *model.EventForwardRule) {
	h.mu.Lock()
	if cancel, ok := h.cancels[rule.ID]; ok {
		cancel()
		delete(h.cancels, rule.ID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancels[rule.ID] = cancel
	h.mu.Unlock()

	ruleCopy := *rule
	go h.watchEvents(ctx, &ruleCopy)
}

func (h *EventForwardHandler) watchEvents(ctx context.Context, rule *model.EventForwardRule) {
	defer h.stopWatcher(rule.ID)

	client, err := k8s.Manager.GetClient(rule.ClusterID)
	if err != nil {
		h.logger.Warn("event-forward watcher: cluster not connected",
			zap.Uint("rule_id", rule.ID),
			zap.Uint("cluster_id", rule.ClusterID),
			zap.Error(err),
		)
		return
	}

	var namespaces []string
	var resources []string
	var eventTypes []string
	var reasons []string

	_ = json.Unmarshal([]byte(rule.Namespaces), &namespaces)
	_ = json.Unmarshal([]byte(rule.Resources), &resources)
	_ = json.Unmarshal([]byte(rule.EventTypes), &eventTypes)
	_ = json.Unmarshal([]byte(rule.Reasons), &reasons)

	watcher, err := client.Clientset.CoreV1().Events("").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		h.logger.Warn("event-forward watcher: watch failed",
			zap.Uint("rule_id", rule.ID),
			zap.Error(err),
		)
		return
	}
	defer watcher.Stop()

	h.logger.Info("event-forward watcher running", zap.Uint("rule_id", rule.ID))

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}

			k8sEvent, ok := event.Object.(*corev1.Event)
			if !ok {
				continue
			}
			h.eventsSeen.Add(1)

			if !h.matchEvent(k8sEvent, namespaces, resources, eventTypes, reasons) {
				continue
			}
			h.eventsMatched.Add(1)

			payload := map[string]interface{}{
				"type":       k8sEvent.Type,
				"reason":     k8sEvent.Reason,
				"message":    k8sEvent.Message,
				"namespace":  k8sEvent.Namespace,
				"object":     fmt.Sprintf("%s/%s", k8sEvent.InvolvedObject.Kind, k8sEvent.InvolvedObject.Name),
				"cluster":    rule.ClusterID,
				"first_time": k8sEvent.FirstTimestamp.Time.Format(time.RFC3339),
				"last_time":  k8sEvent.LastTimestamp.Time.Format(time.RFC3339),
				"count":      k8sEvent.Count,
			}

			// Use the in-memory snapshot. UpdateRule/DeleteRule cancel and restart
			// watchers, so per-event DB lookups are unnecessary.
			err := h.sendWebhook(rule.WebhookURL, rule.Headers, payload)

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
				h.forwardFail.Add(1)
			} else {
				log.Status = "success"
				log.StatusCode = 200
				h.forwardOK.Add(1)
			}

			h.db.Create(&log)
		}
	}
}

func (h *EventForwardHandler) matchEvent(event *corev1.Event, namespaces, resources, eventTypes, reasons []string) bool {
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
