package system

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

type userGroupResponse struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       int    `json:"status"`
	MemberCount  int64  `json:"member_count"`
	ClusterCount int64  `json:"cluster_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type groupMemberResponse struct {
	ID       uint   `json:"id"`
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	RealName string `json:"real_name"`
	Status   int    `json:"status"`
}

func (h *Handler) ListUserGroups(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	query := h.db.Model(&model.UserGroup{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if rawStatus := c.Query("status"); rawStatus != "" {
		status, err := strconv.Atoi(rawStatus)
		if err != nil || (status != 0 && status != 1) {
			response.BadRequest(c, "invalid status")
			return
		}
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.InternalError(c, "failed to count user groups")
		return
	}
	var groups []model.UserGroup
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&groups).Error; err != nil {
		response.InternalError(c, "failed to list user groups")
		return
	}
	result := make([]userGroupResponse, 0, len(groups))
	for _, group := range groups {
		item, err := h.userGroupResponse(group)
		if err != nil {
			response.InternalError(c, "failed to load user group counts")
			return
		}
		result = append(result, item)
	}
	response.PageSuccess(c, result, total, page, size)
}

func (h *Handler) GetUserGroup(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}
	var group model.UserGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "user group not found")
			return
		}
		response.InternalError(c, "failed to get user group")
		return
	}
	result, err := h.userGroupResponse(group)
	if err != nil {
		response.InternalError(c, "failed to load user group counts")
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateUserGroup(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required,min=2,max=64"`
		Description string `json:"description" binding:"max=256"`
		Status      *int   `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	status := 1
	if req.Status != nil {
		status = *req.Status
	}
	if status != 0 && status != 1 {
		response.BadRequest(c, "status must be 0 or 1")
		return
	}
	var count int64
	if err := h.db.Model(&model.UserGroup{}).Where("name = ?", req.Name).Count(&count).Error; err != nil {
		response.InternalError(c, "failed to validate user group name")
		return
	}
	if count > 0 {
		response.BadRequest(c, "user group name already exists")
		return
	}
	group := model.UserGroup{
		Name:        req.Name,
		Description: strings.TrimSpace(req.Description),
		Status:      status,
	}
	if err := h.db.Create(&group).Error; err != nil {
		response.InternalError(c, "failed to create user group")
		return
	}
	result, _ := h.userGroupResponse(group)
	response.Created(c, result)
}

func (h *Handler) UpdateUserGroup(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name" binding:"omitempty,min=2,max=64"`
		Description *string `json:"description" binding:"omitempty,max=256"`
		Status      *int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	var group model.UserGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		response.NotFound(c, "user group not found")
		return
	}
	updates := make(map[string]interface{})
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if len(name) < 2 {
			response.BadRequest(c, "name must contain at least 2 non-space characters")
			return
		}
		var count int64
		if err := h.db.Model(&model.UserGroup{}).Where("name = ? AND id <> ?", name, groupID).Count(&count).Error; err != nil {
			response.InternalError(c, "failed to validate user group name")
			return
		}
		if count > 0 {
			response.BadRequest(c, "user group name already exists")
			return
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		if *req.Status != 0 && *req.Status != 1 {
			response.BadRequest(c, "status must be 0 or 1")
			return
		}
		updates["status"] = *req.Status
	}
	if len(updates) > 0 {
		if err := h.db.Model(&group).Updates(updates).Error; err != nil {
			response.InternalError(c, "failed to update user group")
			return
		}
	}
	if err := h.db.First(&group, groupID).Error; err != nil {
		response.InternalError(c, "user group updated but could not be reloaded")
		return
	}
	result, _ := h.userGroupResponse(group)
	response.SuccessWithMessage(c, "user group updated", result)
}

func (h *Handler) DeleteUserGroup(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}
	var group model.UserGroup
	if err := h.db.First(&group, groupID).Error; err != nil {
		response.NotFound(c, "user group not found")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", groupID).Delete(&model.GroupCluster{}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	}); err != nil {
		response.InternalError(c, "failed to delete user group")
		return
	}
	response.SuccessWithMessage(c, "user group deleted", nil)
}

func (h *Handler) ListUserGroupMembers(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok || !h.userGroupExists(c, groupID) {
		return
	}
	members, err := h.listUserGroupMembers(groupID)
	if err != nil {
		response.InternalError(c, "failed to list user group members")
		return
	}
	response.Success(c, members)
}

func (h *Handler) ReplaceUserGroupMembers(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok || !h.userGroupExists(c, groupID) {
		return
	}
	var req struct {
		UserIDs []uint `json:"user_ids" binding:"max=500,dive,required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	seen := make(map[uint]struct{}, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		if _, exists := seen[userID]; exists {
			response.BadRequest(c, "duplicate user id")
			return
		}
		seen[userID] = struct{}{}
	}
	if len(req.UserIDs) > 0 {
		var count int64
		if err := h.db.Model(&model.User{}).Where("id IN ?", req.UserIDs).Count(&count).Error; err != nil {
			response.InternalError(c, "failed to validate users")
			return
		}
		if count != int64(len(req.UserIDs)) {
			response.BadRequest(c, "one or more users do not exist")
			return
		}
	}
	members := make([]model.UserGroupMember, 0, len(req.UserIDs))
	for _, userID := range req.UserIDs {
		members = append(members, model.UserGroupMember{GroupID: groupID, UserID: userID, Status: 1})
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		if len(members) > 0 {
			return tx.Create(&members).Error
		}
		return nil
	}); err != nil {
		response.InternalError(c, "failed to replace user group members")
		return
	}
	result, err := h.listUserGroupMembers(groupID)
	if err != nil {
		response.InternalError(c, "members updated but could not be reloaded")
		return
	}
	response.SuccessWithMessage(c, "user group members updated", result)
}

func parseGroupID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid user group id")
		return 0, false
	}
	return uint(id), true
}

func (h *Handler) userGroupExists(c *gin.Context, groupID uint) bool {
	var count int64
	if err := h.db.Model(&model.UserGroup{}).Where("id = ?", groupID).Count(&count).Error; err != nil {
		response.InternalError(c, "failed to validate user group")
		return false
	}
	if count == 0 {
		response.NotFound(c, "user group not found")
		return false
	}
	return true
}

func (h *Handler) userGroupResponse(group model.UserGroup) (userGroupResponse, error) {
	var memberCount, clusterCount int64
	if err := h.db.Model(&model.UserGroupMember{}).Where("group_id = ?", group.ID).Count(&memberCount).Error; err != nil {
		return userGroupResponse{}, err
	}
	if err := h.db.Model(&model.GroupCluster{}).Where("group_id = ?", group.ID).Count(&clusterCount).Error; err != nil {
		return userGroupResponse{}, err
	}
	return userGroupResponse{
		ID:           group.ID,
		Name:         group.Name,
		Description:  group.Description,
		Status:       group.Status,
		MemberCount:  memberCount,
		ClusterCount: clusterCount,
		CreatedAt:    group.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    group.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

func (h *Handler) listUserGroupMembers(groupID uint) ([]groupMemberResponse, error) {
	var members []model.UserGroupMember
	if err := h.db.Preload("User").Where("group_id = ?", groupID).Order("user_id").Find(&members).Error; err != nil {
		return nil, err
	}
	result := make([]groupMemberResponse, 0, len(members))
	for _, member := range members {
		result = append(result, groupMemberResponse{
			ID:       member.ID,
			UserID:   member.UserID,
			Username: member.User.Username,
			Email:    member.User.Email,
			RealName: member.User.RealName,
			Status:   member.Status,
		})
	}
	return result, nil
}

type groupClusterResponse struct {
	ID                 uint   `json:"id"`
	GroupID            uint   `json:"group_id"`
	ClusterID          uint   `json:"cluster_id"`
	ClusterName        string `json:"cluster_name"`
	ClusterDisplayName string `json:"cluster_display_name"`
	Namespace          string `json:"namespace"`
	PermissionLevel    string `json:"permission_level"`
}

type effectivePermissionSource struct {
	SourceType      string `json:"source_type"`
	SourceID        *uint  `json:"source_id,omitempty"`
	SourceName      string `json:"source_name"`
	PermissionLevel string `json:"permission_level"`
}

type effectiveUserClusterPermission struct {
	ClusterID          uint                        `json:"cluster_id"`
	ClusterName        string                      `json:"cluster_name"`
	ClusterDisplayName string                      `json:"cluster_display_name"`
	Namespace          string                      `json:"namespace"`
	PermissionLevel    string                      `json:"permission_level"`
	Sources            []effectivePermissionSource `json:"sources"`
}

func (h *Handler) ListUserGroupClusters(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok || !h.userGroupExists(c, groupID) {
		return
	}
	result, err := h.listUserGroupClusters(groupID)
	if err != nil {
		response.InternalError(c, "failed to list user group clusters")
		return
	}
	response.Success(c, result)
}

func (h *Handler) ReplaceUserGroupClusters(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok || !h.userGroupExists(c, groupID) {
		return
	}
	var req replaceClusterAssignmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	assignments, err := h.validateClusterAssignments(0, req.Assignments)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupAssignments := make([]model.GroupCluster, 0, len(assignments))
	for _, item := range assignments {
		groupAssignments = append(groupAssignments, model.GroupCluster{
			GroupID:         groupID,
			ClusterID:       item.ClusterID,
			Namespace:       item.Namespace,
			PermissionLevel: item.PermissionLevel,
			Status:          1,
		})
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&model.GroupCluster{}).Error; err != nil {
			return err
		}
		if len(groupAssignments) > 0 {
			return tx.Create(&groupAssignments).Error
		}
		return nil
	}); err != nil {
		response.InternalError(c, "failed to update user group cluster assignments")
		return
	}
	result, err := h.listUserGroupClusters(groupID)
	if err != nil {
		response.InternalError(c, "assignments updated but could not be reloaded")
		return
	}
	response.SuccessWithMessage(c, "user group cluster assignments updated", result)
}

func (h *Handler) GetUserEffectiveClusterPermissions(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	if !h.userExists(userID) {
		response.NotFound(c, "user not found")
		return
	}

	type grantRow struct {
		ClusterID          uint
		ClusterName        string
		ClusterDisplayName string
		Namespace          string
		PermissionLevel    string
		SourceType         string
		SourceID           *uint
		SourceName         string
	}

	var rows []grantRow
	directSQL := `
SELECT uc.cluster_id, c.name AS cluster_name, c.display_name AS cluster_display_name,
       uc.namespace, uc.permission_level, 'direct' AS source_type, NULL AS source_id, 'direct' AS source_name
FROM user_clusters uc
JOIN clusters c ON c.id = uc.cluster_id AND c.deleted_at IS NULL
WHERE uc.user_id = ?`
	if err := h.db.Raw(directSQL, userID).Scan(&rows).Error; err != nil {
		response.InternalError(c, "failed to load direct cluster permissions")
		return
	}

	var inherited []grantRow
	inheritedSQL := `
SELECT gc.cluster_id, c.name AS cluster_name, c.display_name AS cluster_display_name,
       gc.namespace, gc.permission_level, 'user_group' AS source_type, ug.id AS source_id, ug.name AS source_name
FROM group_clusters gc
JOIN user_groups ug ON ug.id = gc.group_id AND ug.status = 1 AND ug.deleted_at IS NULL
JOIN user_group_members ugm ON ugm.group_id = ug.id AND ugm.status = 1
JOIN clusters c ON c.id = gc.cluster_id AND c.deleted_at IS NULL
WHERE ugm.user_id = ? AND gc.status = 1`
	if err := h.db.Raw(inheritedSQL, userID).Scan(&inherited).Error; err != nil {
		response.InternalError(c, "failed to load inherited cluster permissions")
		return
	}
	rows = append(rows, inherited...)

	type key struct {
		clusterID uint
		namespace string
	}
	merged := make(map[key]*effectiveUserClusterPermission)
	rank := map[string]int{"read": 1, "write": 2, "admin": 3}
	for _, row := range rows {
		namespace := strings.TrimSpace(row.Namespace)
		if namespace == "" {
			namespace = "*"
		}
		k := key{clusterID: row.ClusterID, namespace: namespace}
		item, ok := merged[k]
		if !ok {
			item = &effectiveUserClusterPermission{
				ClusterID:          row.ClusterID,
				ClusterName:        row.ClusterName,
				ClusterDisplayName: row.ClusterDisplayName,
				Namespace:          namespace,
				PermissionLevel:    row.PermissionLevel,
				Sources:            []effectivePermissionSource{},
			}
			merged[k] = item
		} else if rank[row.PermissionLevel] > rank[item.PermissionLevel] {
			item.PermissionLevel = row.PermissionLevel
		}
		item.Sources = append(item.Sources, effectivePermissionSource{
			SourceType:      row.SourceType,
			SourceID:        row.SourceID,
			SourceName:      row.SourceName,
			PermissionLevel: row.PermissionLevel,
		})
	}

	result := make([]effectiveUserClusterPermission, 0, len(merged))
	for _, item := range merged {
		result = append(result, *item)
	}
	response.Success(c, result)
}

func (h *Handler) listUserGroupClusters(groupID uint) ([]groupClusterResponse, error) {
	var assignments []model.GroupCluster
	if err := h.db.Preload("Cluster").Where("group_id = ?", groupID).Order("cluster_id, namespace").Find(&assignments).Error; err != nil {
		return nil, err
	}
	result := make([]groupClusterResponse, 0, len(assignments))
	for _, assignment := range assignments {
		result = append(result, groupClusterResponse{
			ID:                 assignment.ID,
			GroupID:            assignment.GroupID,
			ClusterID:          assignment.ClusterID,
			ClusterName:        assignment.Cluster.Name,
			ClusterDisplayName: assignment.Cluster.DisplayName,
			Namespace:          assignment.Namespace,
			PermissionLevel:    assignment.PermissionLevel,
		})
	}
	return result, nil
}
