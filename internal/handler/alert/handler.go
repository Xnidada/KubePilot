package alert

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
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
		Name       string   `json:"name" binding:"required"`
		ClusterID  uint     `json:"cluster_id" binding:"required"`
		Namespace  string   `json:"namespace"`
		Resource   string   `json:"resource"`
		Metric     string   `json:"metric" binding:"required"`
		Condition  string   `json:"condition" binding:"required"`
		Threshold  float64  `json:"threshold" binding:"required"`
		Duration   string   `json:"duration"`
		Channels   []uint   `json:"channels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
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
		Enabled:   true,
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
		Name      string  `json:"name"`
		Metric    string  `json:"metric"`
		Condition string  `json:"condition"`
		Threshold float64 `json:"threshold"`
		Duration  string  `json:"duration"`
		Enabled   *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Metric != "" {
		updates["metric"] = req.Metric
	}
	if req.Condition != "" {
		updates["condition"] = req.Condition
	}
	if req.Threshold != 0 {
		updates["threshold"] = req.Threshold
	}
	if req.Duration != "" {
		updates["duration"] = req.Duration
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
	err := query.Preload("Rule").Preload("Cluster").Offset((page - 1) * size).Limit(size).Order("id desc").Find(&history).Error
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
		Name   string `json:"name" binding:"required"`
		Type   string `json:"type" binding:"required"`
		Config string `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	channel := &model.NotificationChannel{
		Name:    req.Name,
		Type:    req.Type,
		Config:  req.Config,
		Enabled: true,
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
	if req.Config != "" {
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
