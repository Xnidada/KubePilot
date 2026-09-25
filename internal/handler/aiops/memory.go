package aiops

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/service/aiops"
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

func (h *Handler) ListMemoryAudits(c *gin.Context) {
	uid := c.MustGet("user_id").(uint)
	q := h.db.Model(&model.AgentMemoryAudit{})
	if !h.canBrowseAllConversations(c) {
		q = q.Where("actor_id = ?", uid)
	}
	var rows []model.AgentMemoryAudit
	if err := q.Order("id DESC").Limit(100).Find(&rows).Error; err != nil {
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
		Source     string     `json:"source"`
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
	if !aiops.ValidMemoryContent(req.Content) {
		response.BadRequest(c, "content is empty, too long or appears to contain credentials")
		return
	}
	if req.Type == "verified_knowledge" && (strings.TrimSpace(req.Source) == "" || !aiops.ValidMemoryContent("reviewed:"+strings.TrimSpace(req.Source))) {
		response.BadRequest(c, "verified knowledge requires a non-sensitive verification source")
		return
	}
	if req.Source != "" && !aiops.ValidMemoryContent(req.Source) {
		response.BadRequest(c, "source is too long or appears to contain credentials")
		return
	}
	if req.Type == "verified_knowledge" && req.ExpiresAt == nil {
		expires := time.Now().Add(30 * 24 * time.Hour)
		req.ExpiresAt = &expires
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		response.BadRequest(c, "expires_at must be in the future")
		return
	}
	if req.Type == "verified_knowledge" && req.ExpiresAt != nil && req.ExpiresAt.After(time.Now().Add(90*24*time.Hour)) {
		response.BadRequest(c, "verified knowledge TTL cannot exceed 90 days")
		return
	}
	if req.Confidence <= 0 {
		req.Confidence = 0.8
	}
	if req.Confidence > 1 {
		req.Confidence = 1
	}
	source := strings.TrimSpace(req.Source)
	if req.Type == "verified_knowledge" {
		source = "reviewed:" + source
	} else if source == "" {
		source = "manual"
	}
	m := model.AgentMemory{UserID: uid, ClusterID: req.ClusterID, Type: req.Type, Content: strings.TrimSpace(req.Content), Source: source, Confidence: req.Confidence, ExpiresAt: req.ExpiresAt}
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		return tx.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: uid, Action: "create", Detail: "manual"}).Error
	}); err != nil {
		response.InternalError(c, err.Error())
		return
	}
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
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(m).Error; err != nil {
			return err
		}
		return tx.Create(&model.AgentMemoryAudit{MemoryID: m.ID, ActorID: uid, Action: "pin", Detail: strconv.FormatBool(m.IsPinned)}).Error
	}); err != nil {
		response.InternalError(c, "failed to update memory")
		return
	}
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

// ListMetricReviewSamples exposes a small, recent sample to AI-settings
// editors for manual accuracy assessment. Unreviewed rows do not count as 0%.
func (h *Handler) ListMetricReviewSamples(c *gin.Context) {
	var rows []model.AgentRunMetric
	if err := h.db.Where("assistant_message_id > 0 AND error_assertion_reviewed = ?", false).
		Order("id DESC").Limit(20).Find(&rows).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	type sample struct {
		ID             uint   `json:"id"`
		ConversationID uint   `json:"conversation_id"`
		Question       string `json:"question"`
		Answer         string `json:"answer"`
		Evidence       string `json:"evidence"`
	}
	out := make([]sample, 0, len(rows))
	for _, row := range rows {
		var answer, question model.ChatMessage
		if h.db.Where("id = ? AND conversation_id = ?", row.AssistantMessageID, row.ConversationID).First(&answer).Error != nil {
			continue // The conversation may have been cleared.
		}
		_ = h.db.Where("conversation_id = ? AND id < ? AND role = ?", row.ConversationID, row.AssistantMessageID, "user").Order("id DESC").First(&question).Error
		var extras aiops.MessageExtras
		_ = json.Unmarshal([]byte(answer.Extras), &extras)
		lines := make([]string, 0, 4)
		for _, tool := range extras.ToolTrace {
			if tool.IsError || len(lines) == 4 {
				continue
			}
			result := tool.Result
			if !aiops.ValidMemoryContent(result) {
				result = "[结果过长或含敏感信息；请查看原会话工具轨迹]"
			}
			lines = append(lines, tool.Name+": "+result)
		}
		out = append(out, sample{ID: row.ID, ConversationID: row.ConversationID, Question: reviewExcerpt(question.Content), Answer: reviewExcerpt(answer.Content), Evidence: strings.Join(lines, "\n")})
	}
	response.Success(c, out)
}

func reviewExcerpt(content string) string {
	runes := []rune(content)
	if len(runes) > 2000 {
		return string(runes[:2000]) + "…[截断；请查看原会话]"
	}
	return content
}

func (h *Handler) ReviewMetricAssertion(c *gin.Context) {
	var req struct {
		ErrorAssertion *bool `json:"error_assertion" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ErrorAssertion == nil {
		response.BadRequest(c, "error_assertion must be true or false")
		return
	}
	actor := c.MustGet("user_id").(uint)
	now := time.Now()
	result := h.db.Model(&model.AgentRunMetric{}).
		Where("id = ? AND assistant_message_id > 0 AND error_assertion_reviewed = ?", c.Param("id"), false).
		Updates(map[string]interface{}{"error_assertion": *req.ErrorAssertion, "error_assertion_reviewed": true, "reviewed_by": actor, "reviewed_at": now})
	if result.Error != nil {
		response.InternalError(c, result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		response.NotFound(c, "unreviewed sample not found")
		return
	}
	response.Success(c, gin.H{"reviewed": true})
}
