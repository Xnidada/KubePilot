package system

import (
	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	accessservice "github.com/kubepilot/kubepilot/internal/service/access"
)

type effectiveGrantResponse struct {
	ClusterID          uint                          `json:"cluster_id"`
	ClusterName        string                        `json:"cluster_name"`
	ClusterDisplayName string                        `json:"cluster_display_name"`
	Namespace          string                        `json:"namespace"`
	PermissionLevel    accessservice.PermissionLevel `json:"permission_level"`
}

// PreviewUserEffectiveAccess returns merged direct and inherited grants.
// An application-level admin bypass remains the caller's responsibility.
func (h *Handler) PreviewUserEffectiveAccess(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	if !h.userExists(userID) {
		response.NotFound(c, "user not found")
		return
	}
	effective, err := accessservice.NewService(h.db).Effective(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "failed to calculate effective access")
		return
	}
	grants := effective.Grants()
	clusterIDs := effective.ClusterIDs(accessservice.PermissionRead)
	var clusters []model.Cluster
	if len(clusterIDs) > 0 {
		if err := h.db.Where("id IN ?", clusterIDs).Find(&clusters).Error; err != nil {
			response.InternalError(c, "failed to load cluster details")
			return
		}
	}
	clusterByID := make(map[uint]model.Cluster, len(clusters))
	for _, cluster := range clusters {
		clusterByID[cluster.ID] = cluster
	}
	result := make([]effectiveGrantResponse, 0, len(grants))
	for _, grant := range grants {
		cluster := clusterByID[grant.ClusterID]
		result = append(result, effectiveGrantResponse{
			ClusterID:          grant.ClusterID,
			ClusterName:        cluster.Name,
			ClusterDisplayName: cluster.DisplayName,
			Namespace:          grant.Namespace,
			PermissionLevel:    grant.PermissionLevel,
		})
	}
	response.Success(c, gin.H{
		"user_id":     userID,
		"cluster_ids": clusterIDs,
		"grants":      result,
	})
}
