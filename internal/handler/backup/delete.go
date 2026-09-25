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
	errBackupDeleteNotFound   = errors.New("backup record not found")
	errBackupDeleteForbidden  = errors.New("cluster access denied")
	errBackupDeleteActive     = errors.New("an active backup cannot be deleted")
	errBackupDeleteReferenced = errors.New("a backup referenced by a restore record cannot be deleted")
	errBackupDeleteConflict   = errors.New("backup records changed; refresh and retry")
)

// DeleteBackupRecord deletes one platform record, not the external backup artifact.
func (h *Handler) DeleteBackupRecord(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid backup record id")
		return
	}
	h.deleteBackupRecords(c, []uint{uint(id)})
}

// BatchDeleteBackupRecords deletes only the records explicitly selected by ID.
func (h *Handler) BatchDeleteBackupRecords(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid backup record ids")
		return
	}
	h.deleteBackupRecords(c, req.IDs)
}

func (h *Handler) deleteBackupRecords(c *gin.Context, ids []uint) {
	ids = uniqueBackupIDs(ids)
	var deleted int64
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var records []model.BackupRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Find(&records).Error; err != nil {
			return err
		}
		if len(records) != len(ids) {
			return errBackupDeleteNotFound
		}

		for _, record := range records {
			if err := authz.RequireScope(c, "backups", "delete", record.ClusterID, "*"); err != nil {
				return errBackupDeleteForbidden
			}
			if backupRecordActive(record.Status) {
				return fmt.Errorf("%w: #%d", errBackupDeleteActive, record.ID)
			}
		}

		var referenced []uint
		if err := tx.Model(&model.RestoreRecord{}).Where("backup_id IN ?", ids).Limit(1).Pluck("backup_id", &referenced).Error; err != nil {
			return err
		}
		if len(referenced) > 0 {
			return fmt.Errorf("%w: #%d", errBackupDeleteReferenced, referenced[0])
		}

		result := tx.Where("id IN ?", ids).Delete(&model.BackupRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return errBackupDeleteConflict
		}
		deleted = result.RowsAffected
		return nil
	})

	switch {
	case errors.Is(err, errBackupDeleteNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errBackupDeleteForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, errBackupDeleteActive), errors.Is(err, errBackupDeleteReferenced), errors.Is(err, errBackupDeleteConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": deleted})
	}
}

func uniqueBackupIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func backupRecordActive(status string) bool {
	return status == "pending" || status == "in_progress"
}
