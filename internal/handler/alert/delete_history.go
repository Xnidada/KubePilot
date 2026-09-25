package alert

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
	errAlertHistoryNotFound  = errors.New("alert history not found")
	errAlertHistoryForbidden = errors.New("cluster access denied")
	errAlertHistoryConflict  = errors.New("alert history changed; refresh and retry")
)

func (h *Handler) DeleteAlertHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid alert history id")
		return
	}
	h.deleteAlertHistory(c, []uint{uint(id)})
}

func (h *Handler) BatchDeleteAlertHistory(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid alert history ids")
		return
	}
	h.deleteAlertHistory(c, req.IDs)
}

func (h *Handler) deleteAlertHistory(c *gin.Context, ids []uint) {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var rows []model.AlertHistory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", unique).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(unique) {
			return errAlertHistoryNotFound
		}
		for _, row := range rows {
			if row.ClusterID > 0 {
				if err := authz.RequireScope(c, "alerts", "delete", row.ClusterID, "*"); err != nil {
					return errAlertHistoryForbidden
				}
			}
		}
		result := tx.Where("id IN ?", unique).Delete(&model.AlertHistory{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(unique)) {
			return errAlertHistoryConflict
		}
		return nil
	})
	switch {
	case errors.Is(err, errAlertHistoryNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errAlertHistoryForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, errAlertHistoryConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": len(unique)})
	}
}
