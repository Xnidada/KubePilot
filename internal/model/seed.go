package model

import (
	"fmt"

	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
)

// SeedData 初始化默认数据（角色、权限、用户）
func SeedData() error {
	// 定义默认角色
	type RoleDef struct {
		Name        string
		Description string
		Permissions string
		IsSystem    bool
	}

	roles := []RoleDef{
		{
			Name:        "admin",
			Description: "系统管理员，拥有全部权限",
			Permissions: `[{"resource":"*","actions":["*"]}]`,
			IsSystem:    true,
		},
		{
			Name:        "operator",
			Description: "运维人员，管理工作负载和告警",
			Permissions: `[{"resource":"deployments","actions":["view","create","edit","delete"]},{"resource":"pods","actions":["view","create","delete"]},{"resource":"services","actions":["view","create","edit","delete"]},{"resource":"configmaps","actions":["view","create","edit","delete"]},{"resource":"secrets","actions":["view","create","edit","delete"]},{"resource":"namespaces","actions":["view"]},{"resource":"nodes","actions":["view"]},{"resource":"events","actions":["view"]},{"resource":"alerts","actions":["view","edit"]},{"resource":"scheduler","actions":["view","create","edit","delete"]}]`,
			IsSystem:    false,
		},
		{
			Name:        "user",
			Description: "开发人员，查看和创建工作负载",
			Permissions: `[{"resource":"deployments","actions":["view","create"]},{"resource":"pods","actions":["view"]},{"resource":"services","actions":["view"]},{"resource":"configmaps","actions":["view"]},{"resource":"namespaces","actions":["view"]},{"resource":"scheduler","actions":["view","create"]}]`,
			IsSystem:    false,
		},
		{
			Name:        "viewer",
			Description: "只读用户，仅查看资源",
			Permissions: `[{"resource":"*","actions":["view"]}]`,
			IsSystem:    false,
		},
	}

	// 创建或更新角色
	roleMap := make(map[string]uint)
	for _, r := range roles {
		var existingRole Role
		result := DB.Where("name = ?", r.Name).First(&existingRole)
		if result.Error != nil {
			newRole := Role{
				Name:        r.Name,
				Description: r.Description,
				Permissions: r.Permissions,
				IsSystem:    r.IsSystem,
			}
			if err := DB.Create(&newRole).Error; err != nil {
				logger.Error("failed to create role", zap.String("role", r.Name), zap.Error(err))
			} else {
				logger.Info("role created", zap.String("role", r.Name))
				roleMap[r.Name] = newRole.ID
			}
		} else {
			roleMap[r.Name] = existingRole.ID
			// 强制更新权限
			DB.Model(&existingRole).Update("permissions", r.Permissions)
			DB.Model(&existingRole).Update("description", r.Description)
			DB.Model(&existingRole).Update("is_system", r.IsSystem)
			logger.Info("role updated", zap.String("role", r.Name))
		}
	}

	// 默认密码
	defaultPassword := "admin123"
	hashedPassword, err := crypto.HashPassword(defaultPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// 定义默认用户
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

	// 创建或更新用户
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

	return nil
}
