package system

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"
)

type clusterAssignmentInput struct {
	ClusterID       uint   `json:"cluster_id" binding:"required"`
	Namespace       string `json:"namespace"`
	PermissionLevel string `json:"permission_level" binding:"required"`
}

type replaceClusterAssignmentsRequest struct {
	Assignments []clusterAssignmentInput `json:"assignments" binding:"max=500,dive"`
}

type clusterAssignmentResponse struct {
	ID                 uint   `json:"id"`
	ClusterID          uint   `json:"cluster_id"`
	ClusterName        string `json:"cluster_name"`
	ClusterDisplayName string `json:"cluster_display_name"`
	Namespace          string `json:"namespace"`
	PermissionLevel    string `json:"permission_level"`
}

// ListUserClusters returns all cluster and namespace grants for a user.
func (h *Handler) ListUserClusters(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	if !h.userExists(userID) {
		response.NotFound(c, "user not found")
		return
	}

	assignments, err := h.listUserClusterAssignments(userID)
	if err != nil {
		response.InternalError(c, "failed to list user cluster assignments")
		return
	}
	response.Success(c, assignments)
}

// ReplaceUserClusters atomically replaces all cluster and namespace grants for a user.
func (h *Handler) ReplaceUserClusters(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	if !h.userExists(userID) {
		response.NotFound(c, "user not found")
		return
	}

	var req replaceClusterAssignmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	assignments, err := h.validateClusterAssignments(userID, req.Assignments)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserCluster{}).Error; err != nil {
			return err
		}
		if len(assignments) > 0 {
			return tx.Create(&assignments).Error
		}
		return nil
	})
	if err != nil {
		response.InternalError(c, "failed to update user cluster assignments")
		return
	}

	result, err := h.listUserClusterAssignments(userID)
	if err != nil {
		response.InternalError(c, "assignments updated but could not be reloaded")
		return
	}
	response.SuccessWithMessage(c, "user cluster assignments updated", result)
}

func parseUserID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return 0, false
	}
	return uint(id), true
}

func (h *Handler) userExists(userID uint) bool {
	var count int64
	return h.db.Model(&model.User{}).Where("id = ?", userID).Count(&count).Error == nil && count == 1
}

func (h *Handler) validateClusterAssignments(userID uint, inputs []clusterAssignmentInput) ([]model.UserCluster, error) {
	clusterIDs := make([]uint, 0, len(inputs))
	uniqueClusters := make(map[uint]struct{})
	seen := make(map[string]struct{}, len(inputs))
	assignments := make([]model.UserCluster, 0, len(inputs))

	for _, input := range inputs {
		namespace := strings.TrimSpace(input.Namespace)
		if namespace == "" {
			namespace = "*"
		}
		if namespace != "*" {
			if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
				return nil, fmt.Errorf("invalid namespace %q: %s", namespace, strings.Join(errs, ", "))
			}
		}

		switch input.PermissionLevel {
		case "read", "write", "admin":
		default:
			return nil, fmt.Errorf("invalid permission level %q", input.PermissionLevel)
		}

		key := fmt.Sprintf("%d/%s", input.ClusterID, namespace)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate assignment for cluster %d and namespace %q", input.ClusterID, namespace)
		}
		seen[key] = struct{}{}

		if _, exists := uniqueClusters[input.ClusterID]; !exists {
			uniqueClusters[input.ClusterID] = struct{}{}
			clusterIDs = append(clusterIDs, input.ClusterID)
		}
		assignments = append(assignments, model.UserCluster{
			UserID:          userID,
			ClusterID:       input.ClusterID,
			Namespace:       namespace,
			PermissionLevel: input.PermissionLevel,
		})
	}

	if len(clusterIDs) > 0 {
		var count int64
		if err := h.db.Model(&model.Cluster{}).Where("id IN ?", clusterIDs).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("failed to validate clusters")
		}
		if count != int64(len(clusterIDs)) {
			return nil, fmt.Errorf("one or more clusters do not exist")
		}
	}

	return assignments, nil
}

func (h *Handler) listUserClusterAssignments(userID uint) ([]clusterAssignmentResponse, error) {
	var assignments []model.UserCluster
	if err := h.db.Preload("Cluster").
		Where("user_id = ?", userID).
		Order("cluster_id, namespace").
		Find(&assignments).Error; err != nil {
		return nil, err
	}

	result := make([]clusterAssignmentResponse, 0, len(assignments))
	for _, assignment := range assignments {
		result = append(result, clusterAssignmentResponse{
			ID:                 assignment.ID,
			ClusterID:          assignment.ClusterID,
			ClusterName:        assignment.Cluster.Name,
			ClusterDisplayName: assignment.Cluster.DisplayName,
			Namespace:          assignment.Namespace,
			PermissionLevel:    assignment.PermissionLevel,
		})
	}
	return result, nil
}
