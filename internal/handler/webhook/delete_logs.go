package webhook

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

var (
	errWebhookLogNotFound = errors.New("webhook log not found")
	errWebhookLogRecent   = errors.New("webhook logs from the last 24 hours cannot be deleted")
)

func (h *Handler) DeleteWebhookLog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid webhook log id")
		return
	}
	h.deleteWebhookLogs(c, []uint{uint(id)})
}

func (h *Handler) BatchDeleteWebhookLogs(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid webhook log ids")
		return
	}
	h.deleteWebhookLogs(c, req.IDs)
}

func (h *Handler) deleteWebhookLogs(c *gin.Context, ids []uint) {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var rows []model.WebhookLog
		if err := tx.Where("id IN ?", unique).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(unique) {
			return errWebhookLogNotFound
		}
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, row := range rows {
			if row.CreatedAt.After(cutoff) {
				return errWebhookLogRecent
			}
		}
		result := tx.Where("id IN ? AND created_at <= ?", unique, cutoff).Delete(&model.WebhookLog{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(unique)) {
			return errWebhookLogRecent
		}
		return nil
	})
	switch {
	case errors.Is(err, errWebhookLogNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errWebhookLogRecent):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": len(unique)})
	}
}
