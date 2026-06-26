package system

import (
	"crypto/rand"
	"math/big"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// ListUsers 获取用户列表
func (h *Handler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	var users []model.User
	var total int64

	h.db.Model(&model.User{}).Count(&total)
	err := h.db.Preload("Role").Offset((page - 1) * size).Limit(size).Order("id desc").Find(&users).Error
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type UserInfo struct {
		ID        uint   `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		RealName  string `json:"real_name"`
		Phone     string `json:"phone"`
		Status    int    `json:"status"`
		RoleID    uint   `json:"role_id"`
		RoleName  string `json:"role_name"`
		LastLogin string `json:"last_login"`
		CreatedAt string `json:"created_at"`
	}

	result := make([]UserInfo, 0, len(users))
	for _, u := range users {
		info := UserInfo{
			ID:       u.ID,
			Username: u.Username,
			Email:    u.Email,
			RealName: u.RealName,
			Phone:    u.Phone,
			Status:   u.Status,
			RoleID:   u.RoleID,
			RoleName: u.Role.Name,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if u.LastLogin != nil {
			info.LastLogin = u.LastLogin.Format("2006-01-02 15:04:05")
		}
		result = append(result, info)
	}

	response.PageSuccess(c, result, total, page, size)
}

// GetUser 获取用户详情
func (h *Handler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var user model.User
	if err := h.db.Preload("Role").First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.Success(c, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"real_name":  user.RealName,
		"phone":      user.Phone,
		"status":     user.Status,
		"role_id":    user.RoleID,
		"role_name":  user.Role.Name,
		"last_login": user.LastLogin,
		"created_at": user.CreatedAt,
	})
}

// CreateUser 创建用户
func (h *Handler) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=64"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
		RealName string `json:"real_name"`
		Phone    string `json:"phone"`
		RoleID   uint   `json:"role_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 检查用户名是否已存在
	var count int64
	h.db.Model(&model.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		response.BadRequest(c, "username already exists")
		return
	}

	// 检查邮箱是否已存在
	h.db.Model(&model.User{}).Where("email = ?", req.Email).Count(&count)
	if count > 0 {
		response.BadRequest(c, "email already exists")
		return
	}

	// 检查角色是否存在
	var role model.Role
	if err := h.db.First(&role, req.RoleID).Error; err != nil {
		response.BadRequest(c, "role not found")
		return
	}

	hashedPassword, err := crypto.HashPassword(req.Password)
	if err != nil {
		response.InternalError(c, "failed to hash password")
		return
	}

	user := &model.User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
		RealName: req.RealName,
		Phone:    req.Phone,
		Status:   1,
		RoleID:   req.RoleID,
	}

	if err := h.db.Create(user).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, map[string]interface{}{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	})
}

// UpdateUser 更新用户
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var req struct {
		Email    string `json:"email"`
		RealName string `json:"real_name"`
		Phone    string `json:"phone"`
		RoleID   *uint  `json:"role_id"`
		Status   *int   `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var user model.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	updates := map[string]interface{}{}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.RealName != "" {
		updates["real_name"] = req.RealName
	}
	if req.Phone != "" {
		updates["phone"] = req.Phone
	}
	if req.RoleID != nil {
		updates["role_id"] = *req.RoleID
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "user updated", nil)
}

// DeleteUser 删除用户
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	// 不允许删除管理员
	var user model.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}
	if user.Username == "admin" {
		response.BadRequest(c, "cannot delete admin user")
		return
	}

	if err := h.db.Delete(&model.User{}, id).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "user deleted", nil)
}

// ResetPassword 重置密码
func (h *Handler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}

	var user model.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.NotFound(c, "user not found")
		return
	}

	// 解析请求体中的新密码
	var req struct {
		NewPassword string `json:"new_password"`
	}

	// 尝试解析请求体，如果没有则生成随机密码
	newPassword := ""
	if err := c.ShouldBindJSON(&req); err == nil && req.NewPassword != "" {
		newPassword = req.NewPassword
	}

	if newPassword == "" {
		// 生成随机临时密码
		newPassword = generateRandomPassword(12)
	}

	hashedPassword, err := crypto.HashPassword(newPassword)
	if err != nil {
		response.InternalError(c, "failed to hash password")
		return
	}

	if err := h.db.Model(&user).Update("password", hashedPassword).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"message": "password reset successfully",
	})
}

// ListRoles 获取角色列表
func (h *Handler) ListRoles(c *gin.Context) {
	var roles []model.Role
	if err := h.db.Find(&roles).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 解析权限
	type RoleInfo struct {
		ID          uint                  `json:"id"`
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Permissions model.PermissionList  `json:"permissions"`
		IsSystem    bool                  `json:"is_system"`
		UserCount   int64                 `json:"user_count"`
		CreatedAt   string                `json:"created_at"`
	}

	result := make([]RoleInfo, 0, len(roles))
	for _, role := range roles {
		permissions, _ := model.ParsePermissions(role.Permissions)

		var userCount int64
		h.db.Model(&model.User{}).Where("role_id = ?", role.ID).Count(&userCount)

		result = append(result, RoleInfo{
			ID:          role.ID,
			Name:        role.Name,
			Description: role.Description,
			Permissions: permissions,
			IsSystem:    role.IsSystem,
			UserCount:   userCount,
			CreatedAt:   role.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	response.Success(c, result)
}

// GetRole 获取角色详情
func (h *Handler) GetRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var role model.Role
	if err := h.db.First(&role, id).Error; err != nil {
		response.NotFound(c, "role not found")
		return
	}

	permissions, _ := model.ParsePermissions(role.Permissions)

	var userCount int64
	h.db.Model(&model.User{}).Where("role_id = ?", role.ID).Count(&userCount)

	response.Success(c, map[string]interface{}{
		"id":          role.ID,
		"name":        role.Name,
		"description": role.Description,
		"permissions": permissions,
		"is_system":   role.IsSystem,
		"user_count":  userCount,
		"created_at":  role.CreatedAt,
	})
}

// CreateRole 创建角色
func (h *Handler) CreateRole(c *gin.Context) {
	var req struct {
		Name        string               `json:"name" binding:"required,min=2,max=64"`
		Description string               `json:"description"`
		Permissions model.PermissionList `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// 检查角色名是否已存在
	var count int64
	h.db.Model(&model.Role{}).Where("name = ?", req.Name).Count(&count)
	if count > 0 {
		response.BadRequest(c, "role name already exists")
		return
	}

	role := &model.Role{
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions.ToJSON(),
		IsSystem:    false,
	}

	if err := h.db.Create(role).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, role)
}

// UpdateRole 更新角色
func (h *Handler) UpdateRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var role model.Role
	if err := h.db.First(&role, id).Error; err != nil {
		response.NotFound(c, "role not found")
		return
	}

	// 系统角色不允许修改名称
	if role.IsSystem {
		var req struct {
			Description string               `json:"description"`
			Permissions model.PermissionList `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request: "+err.Error())
			return
		}

		updates := map[string]interface{}{}
		if req.Description != "" {
			updates["description"] = req.Description
		}
		if req.Permissions != nil {
			updates["permissions"] = req.Permissions.ToJSON()
		}

		if err := h.db.Model(&role).Updates(updates).Error; err != nil {
			response.InternalError(c, err.Error())
			return
		}
	} else {
		var req struct {
			Name        string               `json:"name"`
			Description string               `json:"description"`
			Permissions model.PermissionList `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request: "+err.Error())
			return
		}

		updates := map[string]interface{}{}
		if req.Name != "" {
			updates["name"] = req.Name
		}
		if req.Description != "" {
			updates["description"] = req.Description
		}
		if req.Permissions != nil {
			updates["permissions"] = req.Permissions.ToJSON()
		}

		if err := h.db.Model(&role).Updates(updates).Error; err != nil {
			response.InternalError(c, err.Error())
			return
		}
	}

	response.SuccessWithMessage(c, "role updated", nil)
}

// DeleteRole 删除角色
func (h *Handler) DeleteRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid role id")
		return
	}

	var role model.Role
	if err := h.db.First(&role, id).Error; err != nil {
		response.NotFound(c, "role not found")
		return
	}

	if role.IsSystem {
		response.BadRequest(c, "cannot delete system role")
		return
	}

	// 检查是否有用户使用此角色
	var userCount int64
	h.db.Model(&model.User{}).Where("role_id = ?", role.ID).Count(&userCount)
	if userCount > 0 {
		response.BadRequest(c, "cannot delete role with assigned users")
		return
	}

	if err := h.db.Delete(&role).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "role deleted", nil)
}

// GetResourceTypes 获取资源类型列表
func (h *Handler) GetResourceTypes(c *gin.Context) {
	response.Success(c, model.ResourceTypes)
}

// GetActionTypes 获取操作类型列表
func (h *Handler) GetActionTypes(c *gin.Context) {
	response.Success(c, model.ActionTypes)
}

// GetRoleTemplates 获取角色模板
func (h *Handler) GetRoleTemplates(c *gin.Context) {
	response.Success(c, model.RoleTemplates)
}

// GetAuditLogs 获取审计日志
func (h *Handler) GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	username := c.Query("username")
	action := c.Query("action")
	resource := c.Query("resource")

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	query := h.db.Model(&model.AuditLog{})
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if resource != "" {
		query = query.Where("resource_type LIKE ?", "%"+resource+"%")
	}

	var total int64
	query.Count(&total)

	var logs []model.AuditLog
	err := query.Offset((page - 1) * size).Limit(size).Order("id desc").Find(&logs).Error
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.PageSuccess(c, logs, total, page, size)
}


// generateRandomPassword 生成随机密码
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
