package backup

import (
	"errors"
	"fmt"
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
	errRestoreNotFound  = errors.New("restore record not found")
	errRestoreForbidden = errors.New("cluster access denied")
	errRestoreActive    = errors.New("an active restore cannot be deleted")
	errRestoreConflict  = errors.New("restore records changed; refresh and retry")
)

// DeleteRestoreRecord removes only the platform record, never restored resources.
func (h *Handler) DeleteRestoreRecord(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid restore record id")
		return
	}
	h.deleteRestoreRecords(c, []uint{uint(id)})
}

func (h *Handler) BatchDeleteRestoreRecords(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid restore record ids")
		return
	}
	h.deleteRestoreRecords(c, req.IDs)
}

func (h *Handler) deleteRestoreRecords(c *gin.Context, ids []uint) {
	ids = uniqueBackupIDs(ids)
	var deleted int64
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var records []model.RestoreRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Backup").Where("id IN ?", ids).Find(&records).Error; err != nil {
			return err
		}
		if len(records) != len(ids) {
			return errRestoreNotFound
		}
		for _, record := range records {
			if record.Backup.ID == 0 {
				return fmt.Errorf("%w: backup #%d missing", errRestoreConflict, record.BackupID)
			}
			if err := authz.RequireScope(c, "backups", "delete", record.ClusterID, "*"); err != nil {
				return errRestoreForbidden
			}
			if err := authz.RequireScope(c, "backups", "delete", record.Backup.ClusterID, "*"); err != nil {
				return errRestoreForbidden
			}
			if record.Status != "completed" && record.Status != "failed" {
				return fmt.Errorf("%w: #%d", errRestoreActive, record.ID)
			}
		}
		result := tx.Where("id IN ?", ids).Delete(&model.RestoreRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return errRestoreConflict
		}
		deleted = result.RowsAffected
		return nil
	})
	switch {
	case errors.Is(err, errRestoreNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errRestoreForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, errRestoreActive), errors.Is(err, errRestoreConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": deleted})
	}
}
