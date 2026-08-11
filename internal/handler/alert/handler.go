package alert

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/netutil"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	alertService "github.com/kubepilot/kubepilot/internal/service/alert"
	"gorm.io/gorm"
)

type Handler struct {
	db       *gorm.DB
	notifier *alertService.Notifier
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{
		db:       db,
		notifier: alertService.NewNotifier(db),
	}
}

// ListAlertRules 获取告警规则列表
func (h *Handler) ListAlertRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	clusterID := c.Query("cluster_id")

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	query := h.db.Model(&model.AlertRule{})
	if clusterID != "" {
		query = query.Where("cluster_id = ?", clusterID)
	}

	var total int64
	query.Count(&total)

	var rules []model.AlertRule
	err := query.Preload("Cluster").Offset((page - 1) * size).Limit(size).Order("id desc").Find(&rules).Error
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.PageSuccess(c, rules, total, page, size)
}

// CreateAlertRule 创建告警规则
func (h *Handler) CreateAlertRule(c *gin.Context) {
	var req struct {
		Name      string  `json:"name" binding:"required"`
		ClusterID uint    `json:"cluster_id" binding:"required"`
		Namespace string  `json:"namespace"`
		Resource  string  `json:"resource"`
		Metric    string  `json:"metric" binding:"required"`
		Condition string  `json:"condition" binding:"required"`
		Threshold float64 `json:"threshold" binding:"required"`
		Duration  string  `json:"duration"`
		Channels  []uint  `json:"channels"`
		Enabled   *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	channelsJSON := "[]"
	if req.Channels != nil {
		b, err := json.Marshal(req.Channels)
		if err != nil {
			response.BadRequest(c, "invalid channels: "+err.Error())
			return
		}
		channelsJSON = string(b)
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule := &model.AlertRule{
		Name:      req.Name,
		ClusterID: req.ClusterID,
		Namespace: req.Namespace,
		Resource:  req.Resource,
		Metric:    req.Metric,
		Condition: req.Condition,
		Threshold: req.Threshold,
		Duration:  req.Duration,
		Channels:  channelsJSON,
		Enabled:   enabled,
	}

	if err := h.db.Create(rule).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, rule)
}

// UpdateAlertRule 更新告警规则
func (h *Handler) UpdateAlertRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid rule id")
		return
	}

	var rule model.AlertRule
	if err := h.db.First(&rule, id).Error; err != nil {
		response.NotFound(c, "rule not found")
		return
	}

	var req struct {
		Name      string   `json:"name"`
		Namespace *string  `json:"namespace"`
		Resource  *string  `json:"resource"`
		Metric    string   `json:"metric"`
		Condition string   `json:"condition"`
		Threshold *float64 `json:"threshold"`
		Duration  string   `json:"duration"`
		Channels  []uint   `json:"channels"`
		Enabled   *bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Namespace != nil {
		updates["namespace"] = *req.Namespace
	}
	if req.Resource != nil {
		updates["resource"] = *req.Resource
	}
	if req.Metric != "" {
		updates["metric"] = req.Metric
	}
	if req.Condition != "" {
		updates["condition"] = req.Condition
	}
	if req.Threshold != nil {
		updates["threshold"] = *req.Threshold
	}
	if req.Duration != "" {
		updates["duration"] = req.Duration
	}
	if req.Channels != nil {
		b, err := json.Marshal(req.Channels)
		if err != nil {
			response.BadRequest(c, "invalid channels: "+err.Error())
			return
		}
		updates["channels"] = string(b)
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.db.Model(&rule).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "rule updated", nil)
}

// DeleteAlertRule 删除告警规则
func (h *Handler) DeleteAlertRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid rule id")
		return
	}

	if err := h.db.Delete(&model.AlertRule{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "rule deleted", nil)
}

// ListAlertHistory 获取告警历史
func (h *Handler) ListAlertHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	status := c.Query("status")

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	query := h.db.Model(&model.AlertHistory{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var history []model.AlertHistory
	err := query.Offset((page - 1) * size).Limit(size).Order("id desc").Find(&history).Error
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.PageSuccess(c, history, total, page, size)
}

// ListNotificationChannels 获取通知渠道列表
func (h *Handler) ListNotificationChannels(c *gin.Context) {
	var channels []model.NotificationChannel
	if err := h.db.Find(&channels).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, channels)
}

// CreateNotificationChannel 创建通知渠道
func (h *Handler) CreateNotificationChannel(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Type    string `json:"type" binding:"required"`
		Config  string `json:"config" binding:"required"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if err := validateAlertChannelConfig(req.Type, req.Config); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	channel := &model.NotificationChannel{
		Name:    req.Name,
		Type:    req.Type,
		Config:  req.Config,
		Enabled: enabled,
	}

	if err := h.db.Create(channel).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, channel)
}

// UpdateNotificationChannel 更新通知渠道
func (h *Handler) UpdateNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid channel id")
		return
	}

	var channel model.NotificationChannel
	if err := h.db.First(&channel, id).Error; err != nil {
		response.NotFound(c, "channel not found")
		return
	}

	var req struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Config  string `json:"config"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Type != "" {
		updates["type"] = req.Type
	}
	if req.Config != "" {
		chType := req.Type
		if chType == "" {
			chType = channel.Type
		}
		if err := validateAlertChannelConfig(chType, req.Config); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		updates["config"] = req.Config
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.db.Model(&channel).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "channel updated", nil)
}

// DeleteNotificationChannel 删除通知渠道
func (h *Handler) DeleteNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid channel id")
		return
	}

	if err := h.db.Delete(&model.NotificationChannel{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "channel deleted", nil)
}

// ReceiveAlertmanager 接收 Alertmanager 兼容 webhook
func (h *Handler) ReceiveAlertmanager(c *gin.Context) {
	var req struct {
		Receiver string `json:"receiver"`
		Status   string `json:"status"`
		Alerts   []struct {
			Status       string            `json:"status"`
			Labels       map[string]string `json:"labels"`
			Annotations  map[string]string `json:"annotations"`
			StartsAt     time.Time         `json:"startsAt"`
			EndsAt       time.Time         `json:"endsAt"`
			GeneratorURL string            `json:"generatorURL"`
		} `json:"alerts"`
		ChannelIDs []uint `json:"channel_ids"`
		ChannelID  *uint  `json:"channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if len(req.Alerts) == 0 {
		response.BadRequest(c, "alerts is empty")
		return
	}

	channelIDs := req.ChannelIDs
	if req.ChannelID != nil {
		channelIDs = append(channelIDs, *req.ChannelID)
	}
	if q := c.Query("channel_id"); q != "" {
		if id, err := strconv.ParseUint(q, 10, 32); err == nil {
			channelIDs = append(channelIDs, uint(id))
		}
	}

	now := time.Now()
	created := 0
	for _, a := range req.Alerts {
		status := a.Status
		if status == "" {
			status = req.Status
		}
		if status == "" {
			status = "firing"
		}

		alertName := a.Labels["alertname"]
		if alertName == "" {
			alertName = "alertmanager"
		}
		namespace := a.Labels["namespace"]
		resource := a.Labels["pod"]
		if resource == "" {
			resource = a.Labels["instance"]
		}
		if resource == "" {
			resource = a.Labels["job"]
		}

		message := a.Annotations["summary"]
		if message == "" {
			message = a.Annotations["description"]
		}
		if message == "" {
			message = "Alertmanager alert: " + alertName
		}

		triggeredAt := a.StartsAt
		if triggeredAt.IsZero() {
			triggeredAt = now
		}

		history := &model.AlertHistory{
			RuleID:      0,
			Namespace:   namespace,
			Resource:    resource,
			Message:     message,
			Status:      status,
			TriggeredAt: triggeredAt,
		}
		if status == "resolved" && !a.EndsAt.IsZero() {
			t := a.EndsAt
			history.ResolvedAt = &t
		}

		if err := h.db.Omit("Rule", "Cluster").Create(history).Error; err != nil {
			response.InternalError(c, err.Error())
			return
		}
		created++

		title := "[KubePilot] " + alertName
		_ = h.notifier.Notify(c.Request.Context(), channelIDs, title, message, status, history)
	}

	response.Success(c, gin.H{"received": created})
}

func validateAlertChannelConfig(channelType, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var cfg struct {
		WebhookURL string `json:"webhook_url"`
		URL        string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	u := strings.TrimSpace(cfg.WebhookURL)
	if u == "" {
		u = strings.TrimSpace(cfg.URL)
	}
	if u == "" {
		return nil
	}
	if err := netutil.ValidateOutboundURL(u); err != nil {
		return err
	}
	_ = channelType
	return nil
}
