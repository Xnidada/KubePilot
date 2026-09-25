package inspection

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
	errInspectionReportNotFound  = errors.New("inspection report not found")
	errInspectionReportForbidden = errors.New("cluster access denied")
	errInspectionReportRunning   = errors.New("a running inspection report cannot be deleted")
	errInspectionReportConflict  = errors.New("inspection reports changed; refresh and retry")
)

// DeleteReport deletes one finished report together with its result rows.
func (h *InspectionHandler) DeleteReport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid inspection report id")
		return
	}
	h.deleteReports(c, []uint{uint(id)})
}

// BatchDeleteReports deletes only the explicitly selected reports.
func (h *InspectionHandler) BatchDeleteReports(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "select 1 to 100 valid inspection report ids")
		return
	}
	h.deleteReports(c, req.IDs)
}

func (h *InspectionHandler) deleteReports(c *gin.Context, ids []uint) {
	ids = uniqueInspectionReportIDs(ids)
	var deleted int64
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var reports []model.InspectionReport
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Find(&reports).Error; err != nil {
			return err
		}
		if len(reports) != len(ids) {
			return errInspectionReportNotFound
		}
		for _, report := range reports {
			if err := authz.RequireScope(c, "inspection", "delete", report.ClusterID, "*"); err != nil {
				return errInspectionReportForbidden
			}
			if report.Status == "running" {
				return fmt.Errorf("%w: #%d", errInspectionReportRunning, report.ID)
			}
		}

		if err := tx.Where("report_id IN ?", ids).Delete(&model.InspectionResult{}).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ?", ids).Delete(&model.InspectionReport{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return errInspectionReportConflict
		}
		deleted = result.RowsAffected
		return nil
	})

	switch {
	case errors.Is(err, errInspectionReportNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, errInspectionReportForbidden):
		response.Forbidden(c, err.Error())
	case errors.Is(err, errInspectionReportRunning), errors.Is(err, errInspectionReportConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case err != nil:
		response.InternalError(c, err.Error())
	default:
		response.Success(c, gin.H{"deleted": deleted})
	}
}

func uniqueInspectionReportIDs(ids []uint) []uint {
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
