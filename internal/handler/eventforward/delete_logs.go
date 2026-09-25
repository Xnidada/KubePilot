package eventforward

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errForwardLogNotFound  = errors.New("event-forward log not found")
	errForwardLogForbidden = errors.New("cluster access denied")
	errForwardLogConflict  = errors.New("event-forward logs changed; refresh and retry")
)

func (h *EventForwardHandler) DeleteLog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid event-forward log id")
		return
	}
	h.deleteLogs(c, []uint{uint(id)})
}

func (h *EventForwardHandler) BatchDeleteLogs(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid event-forward log ids")
		return
	}
	h.deleteLogs(c, req.IDs)
}

func (h *EventForwardHandler) deleteLogs(c *gin.Context, ids []uint) {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var rows []model.EventForwardLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", unique).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(unique) {
			return errForwardLogNotFound
		}
		for _, row := range rows {
			if err := authz.RequireScope(c, "event_forward", "delete", row.ClusterID, "*"); err != nil {
				return errForwardLogForbidden
			}
		}
		result := tx.Where("id IN ?", unique).Delete(&model.EventForwardLog{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(unique)) {
			return errForwardLogConflict
		}
		return nil
	})
	switch {
	case errors.Is(err, errForwardLogNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errForwardLogForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, errForwardLogConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": len(unique)})
	}
}
