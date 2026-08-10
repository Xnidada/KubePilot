package model

import (
	"fmt"

	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
)

// SeedData initializes default roles and users for a fresh install.
// Existing non-system roles keep administrator customizations.
func SeedData() error {
	type RoleDef struct {
		Name        string
		Description string
		IsSystem    bool
	}

	roles := []RoleDef{
		{Name: "admin", Description: "系统管理员，拥有全部权限", IsSystem: true},
		{Name: "operator", Description: "运维人员，管理工作负载和告警", IsSystem: false},
		{Name: "user", Description: "开发人员，查看和创建工作负载", IsSystem: false},
		{Name: "viewer", Description: "只读用户，仅查看资源", IsSystem: false},
	}

	roleMap := make(map[string]uint)
	for _, r := range roles {
		template, ok := RoleTemplates[r.Name]
		if !ok {
			continue
		}
		permissions := template.ToJSON()

		var existingRole Role
		result := DB.Where("name = ?", r.Name).First(&existingRole)
		if result.Error != nil {
			newRole := Role{
				Name:        r.Name,
				Description: r.Description,
				Permissions: permissions,
				IsSystem:    r.IsSystem,
			}
			if err := DB.Create(&newRole).Error; err != nil {
				logger.Error("failed to create role", zap.String("role", r.Name), zap.Error(err))
			} else {
				logger.Info("role created", zap.String("role", r.Name))
				roleMap[r.Name] = newRole.ID
			}
			continue
		}

		roleMap[r.Name] = existingRole.ID
		updates := map[string]interface{}{
			"description": r.Description,
			"is_system":   r.IsSystem,
		}
		// Only force-sync the immutable admin template. Other roles preserve custom edits.
		if r.IsSystem || r.Name == "admin" {
			updates["permissions"] = permissions
		}
		if err := DB.Model(&existingRole).Updates(updates).Error; err != nil {
			logger.Error("failed to update role", zap.String("role", r.Name), zap.Error(err))
			continue
		}
		logger.Info("role ensured", zap.String("role", r.Name))
	}

	defaultPassword := "admin123"
	hashedPassword, err := crypto.HashPassword(defaultPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	type UserDef struct {
		Username string
		Email    string
		RealName string
		RoleName string
	}
	users := []UserDef{
		{Username: "admin", Email: "admin@kubepilot.io", RealName: "系统管理员", RoleName: "admin"},
		{Username: "operator", Email: "operator@kubepilot.io", RealName: "运维工程师", RoleName: "operator"},
		{Username: "developer", Email: "developer@kubepilot.io", RealName: "开发人员", RoleName: "user"},
		{Username: "viewer", Email: "viewer@kubepilot.io", RealName: "只读用户", RoleName: "viewer"},
	}

	for _, u := range users {
		roleID, ok := roleMap[u.RoleName]
		if !ok {
			continue
		}
		var existingUser User
		result := DB.Where("username = ?", u.Username).First(&existingUser)
		if result.Error != nil {
			newUser := User{
				Username: u.Username,
				Email:    u.Email,
				Password: hashedPassword,
				RealName: u.RealName,
				Status:   1,
				RoleID:   roleID,
			}
			if err := DB.Create(&newUser).Error; err != nil {
				logger.Error("failed to create user", zap.String("user", u.Username), zap.Error(err))
			} else {
				logger.Info("user created", zap.String("user", u.Username), zap.String("role", u.RoleName))
			}
		}
	}

	warnUsersWithoutClusterGrants()
	return nil
}

// warnUsersWithoutClusterGrants logs non-admin users who currently have no
// direct or inherited cluster grants. It does not auto-grant access.
func warnUsersWithoutClusterGrants() {
	type row struct {
		ID       uint
		Username string
		RoleName string
	}
	var users []row
	err := DB.Raw(`
SELECT u.id, u.username, r.name AS role_name
FROM users u
JOIN roles r ON r.id = u.role_id AND r.deleted_at IS NULL
WHERE u.deleted_at IS NULL
  AND u.status = 1
  AND COALESCE(r.is_system, false) = false
  AND r.name <> 'admin'
  AND NOT EXISTS (
    SELECT 1 FROM user_clusters uc WHERE uc.user_id = u.id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM user_group_members ugm
    JOIN user_groups ug ON ug.id = ugm.group_id AND ug.status = 1 AND ug.deleted_at IS NULL
    JOIN group_clusters gc ON gc.group_id = ug.id AND gc.status = 1
    WHERE ugm.user_id = u.id AND ugm.status = 1
  )
`).Scan(&users).Error
	if err != nil {
		logger.Warn("failed to check users without cluster grants", zap.Error(err))
		return
	}
	for _, user := range users {
		logger.Warn("non-admin user has no cluster grants; cluster list will be empty until an administrator assigns access",
			zap.Uint("user_id", user.ID),
			zap.String("username", user.Username),
			zap.String("role", user.RoleName),
		)
	}
}
