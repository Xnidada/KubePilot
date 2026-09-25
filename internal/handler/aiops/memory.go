package aiops

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

func (h *Handler) memoryQuery(c *gin.Context) *gorm.DB {
	uid := c.MustGet("user_id").(uint)
	q := h.db.Model(&model.AgentMemory{}).Order("is_pinned DESC, updated_at DESC")
	if !h.canBrowseAllConversations(c) {
		q = q.Where("user_id = ?", uid)
	}
	return q
}

func (h *Handler) ListMemories(c *gin.Context) {
	q := h.memoryQuery(c)
	if typ := c.Query("type"); typ != "" {
		q = q.Where("type = ?", typ)
	}
	if clusterID, err := strconv.ParseUint(c.Query("cluster_id"), 10, 32); err == nil && clusterID > 0 {
		q = q.Where("cluster_id IS NULL OR cluster_id = ?", uint(clusterID))
	}
	var rows []model.AgentMemory
	if err := q.Find(&rows).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, rows)
}

func (h *Handler) CreateMemory(c *gin.Context) {
	uid := c.MustGet("user_id").(uint)
	var req struct {
		Type       string     `json:"type"`
		Content    string     `json:"content"`
		ClusterID  *uint      `json:"cluster_id"`
		Confidence float64    `json:"confidence"`
		ExpiresAt  *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if req.Type != "preference" && req.Type != "verified_knowledge" {
		response.BadRequest(c, "type must be preference or verified_knowledge")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		response.BadRequest(c, "content is required")
		return
	}
	if req.Confidence <= 0 {
		req.Confidence = 0.8
	}
	if req.Confidence > 1 {
		req.Confidence = 1
	}
	m := model.AgentMemory{UserID: uid, ClusterID: req.ClusterID, Type: req.Type, Content: strings.TrimSpace(req.Content), Source: "manual", Confidence: req.Confidence, ExpiresAt: req.ExpiresAt}
	if err := h.db.Create(&m).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	_ = h.db.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: uid, Action: "create", Detail: "manual"}).Error
	response.Created(c, m)
}

func (h *Handler) ownerMemory(c *gin.Context) (*model.AgentMemory, bool) {
	uid := c.MustGet("user_id").(uint)
	var m model.AgentMemory
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), uid).First(&m).Error; err != nil {
		response.NotFound(c, "memory not found")
		return nil, false
	}
	return &m, true
}
func (h *Handler) PinMemory(c *gin.Context) {
	m, ok := h.ownerMemory(c)
	if !ok {
		return
	}
	uid := c.MustGet("user_id").(uint)
	m.IsPinned = !m.IsPinned
	_ = h.db.Save(m).Error
	_ = h.db.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: uid, Action: "pin", Detail: strconv.FormatBool(m.IsPinned)}).Error
	response.Success(c, m)
}
func (h *Handler) ForgetMemory(c *gin.Context) {
	m, ok := h.ownerMemory(c)
	if !ok {
		return
	}
	uid := c.MustGet("user_id").(uint)
	_ = h.db.Delete(m).Error
	_ = h.db.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: uid, Action: "forget"}).Error
	response.SuccessWithMessage(c, "memory forgotten", nil)
}
func (h *Handler) GetMemoryMetrics(c *gin.Context) {
	if h.service == nil {
		response.InternalError(c, "AI service not configured")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	result, err := h.service.GetMemoryMetricSummary(days)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}
