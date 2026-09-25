package aiops

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid memory id")
		return
	}
	h.forgetMemories(c, []uint{uint(id)})
}

func (h *Handler) BatchForgetMemories(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid memory ids")
		return
	}
	h.forgetMemories(c, req.IDs)
}

func (h *Handler) forgetMemories(c *gin.Context, ids []uint) {
	uid := c.MustGet("user_id").(uint)
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	errNotFound := errors.New("memory not found")
	errPinned := errors.New("unpin memory before forgetting it")
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var rows []model.AgentMemory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ? AND user_id = ?", unique, uid).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(unique) {
			return errNotFound
		}
		audits := make([]model.AgentMemoryAudit, 0, len(rows))
		for _, row := range rows {
			if row.IsPinned {
				return errPinned
			}
			audits = append(audits, model.AgentMemoryAudit{MemoryID: row.ID, ActorID: uid, Action: "forget"})
		}
		result := tx.Where("id IN ? AND user_id = ?", unique, uid).Delete(&model.AgentMemory{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(unique)) {
			return errNotFound
		}
		return tx.Create(&audits).Error
	})
	switch {
	case errors.Is(err, errNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errPinned):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": len(unique)})
	}
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
