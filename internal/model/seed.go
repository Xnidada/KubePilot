package model

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SeedData bootstraps missing roles and an admin. Demo users are opt-in and
// existing users, roles and cluster grants are never rewritten.
func SeedData() error {
	var existingAdmin User
	lookupErr := DB.Where("username = ?", "admin").First(&existingAdmin).Error
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup admin: %w", lookupErr)
	}
	adminMissing := errors.Is(lookupErr, gorm.ErrRecordNotFound)
	adminPassword := os.Getenv("KUBEPILOT_BOOTSTRAP_ADMIN_PASSWORD")
	if adminMissing {
		if err := validateSeedPassword(adminPassword); err != nil {
			return fmt.Errorf("KUBEPILOT_BOOTSTRAP_ADMIN_PASSWORD: %w", err)
		}
	}
	seedDemo := os.Getenv("KUBEPILOT_SEED_DEMO_USERS") == "true"
	demoPassword := os.Getenv("KUBEPILOT_DEMO_PASSWORD")
	if seedDemo {
		if err := validateSeedPassword(demoPassword); err != nil {
			return fmt.Errorf("KUBEPILOT_DEMO_PASSWORD: %w", err)
		}
	}
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
			Permissions: `[{"resource":"deployments","actions":["view","create","edit","delete"]},{"resource":"pods","actions":["view","create","delete","exec"]},{"resource":"services","actions":["view","create","edit","delete"]},{"resource":"configmaps","actions":["view","create","edit","delete"]},{"resource":"secrets","actions":["view","create","edit","delete"]},{"resource":"namespaces","actions":["view"]},{"resource":"nodes","actions":["view"]},{"resource":"events","actions":["view"]},{"resource":"alerts","actions":["view","edit"]},{"resource":"scheduler","actions":["view","create","edit","delete"]}]`,
			IsSystem:    false,
		},
		{
			Name:        "user",
			Description: "开发人员，查看和创建工作负载",
			Permissions: `[{"resource":"deployments","actions":["view","create"]},{"resource":"pods","actions":["view","exec"]},{"resource":"services","actions":["view"]},{"resource":"configmaps","actions":["view"]},{"resource":"namespaces","actions":["view"]},{"resource":"scheduler","actions":["view","create"]}]`,
			IsSystem:    false,
		},
		{
			Name:        "viewer",
			Description: "只读用户，仅查看资源（不含 AI 智能）",
			Permissions: RoleTemplates["viewer"].ToJSON(),
			IsSystem:    false,
		},
		{
			Name:        "aiviewer",
			Description: "只读用户，额外可浏览 AI 智能全部页面（不可执行 AI 操作）",
			Permissions: RoleTemplates["aiviewer"].ToJSON(),
			IsSystem:    false,
		},
	}

	// Create only missing roles; local permission edits must survive re-runs.
	roleMap := make(map[string]uint)
	for _, r := range roles {
		var existingRole Role
		result := DB.Where("name = ?", r.Name).First(&existingRole)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			newRole := Role{
				Name:        r.Name,
				Description: r.Description,
				Permissions: r.Permissions,
				IsSystem:    r.IsSystem,
			}
			if err := DB.Create(&newRole).Error; err != nil {
				return fmt.Errorf("create role %s: %w", r.Name, err)
			} else {
				logger.Info("role created", zap.String("role", r.Name))
				roleMap[r.Name] = newRole.ID
			}
		} else if result.Error != nil {
			return fmt.Errorf("lookup role %s: %w", r.Name, result.Error)
		} else {
			roleMap[r.Name] = existingRole.ID
		}
	}

	// Create the administrator only when absent. Existing credentials stay put.
	type UserDef struct {
		Username string
		Email    string
		RealName string
		RoleName string
	}

	if adminMissing {
		password, err := crypto.HashPassword(adminPassword)
		if err != nil {
			return fmt.Errorf("hash admin password: %w", err)
		}
		admin := User{Username: "admin", Email: "admin@kubepilot.io", Password: password, RealName: "系统管理员", Status: 1, RoleID: roleMap["admin"]}
		if err := DB.Create(&admin).Error; err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
	}
	if !seedDemo {
		return nil
	}
	password, err := crypto.HashPassword(demoPassword)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}
	users := []UserDef{
		{Username: "operator", Email: "operator@kubepilot.io", RealName: "运维工程师", RoleName: "operator"},
		{Username: "developer", Email: "developer@kubepilot.io", RealName: "开发人员", RoleName: "user"},
		{Username: "viewer", Email: "viewer@kubepilot.io", RealName: "只读用户", RoleName: "viewer"},
		{Username: "aiviewer", Email: "aiviewer@kubepilot.io", RealName: "AI 只读用户", RoleName: "aiviewer"},
	}

	// Demo users are never granted cluster access automatically.
	for _, u := range users {
		roleID, ok := roleMap[u.RoleName]
		if !ok {
			continue
		}

		var existingUser User
		result := DB.Where("username = ?", u.Username).First(&existingUser)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			newUser := User{
				Username: u.Username,
				Email:    u.Email,
				Password: password,
				RealName: u.RealName,
				Status:   1,
				RoleID:   roleID,
			}
			if err := DB.Create(&newUser).Error; err != nil {
				return fmt.Errorf("create user %s: %w", u.Username, err)
			} else {
				logger.Info("user created", zap.String("user", u.Username), zap.String("role", u.RoleName))
			}
		} else if result.Error != nil {
			return fmt.Errorf("lookup user %s: %w", u.Username, result.Error)
		}
	}
	return nil
}

func validateSeedPassword(password string) error {
	if len([]rune(password)) < 12 || strings.EqualFold(password, "admin123") {
		return fmt.Errorf("provide a unique password of at least 12 characters")
	}
	return nil
}
