//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output); err != nil {
		fmt.Printf("failed to init logger: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	if err := model.InitDatabase(cfg.Database.Driver, cfg.Database.DSN(), cfg.Database.MaxIdleConns, cfg.Database.MaxOpenConns); err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}

	// Define roles with permissions
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
			Description: "只读用户，仅查看资源",
			Permissions: `[{"resource":"*","actions":["view"]}]`,
			IsSystem:    false,
		},
	}

	// Create or update roles
	roleMap := make(map[string]uint)
	for _, r := range roles {
		var existingRole model.Role
		result := model.DB.Where("name = ?", r.Name).First(&existingRole)
		if result.Error != nil {
			// Role doesn't exist, create it
			newRole := model.Role{
				Name:        r.Name,
				Description: r.Description,
				Permissions: r.Permissions,
				IsSystem:    r.IsSystem,
			}
			if err := model.DB.Create(&newRole).Error; err != nil {
				logger.Error("failed to create role", zap.String("role", r.Name), zap.Error(err))
			} else {
				logger.Info("role created", zap.String("role", r.Name))
				roleMap[r.Name] = newRole.ID
			}
		} else {
			roleMap[r.Name] = existingRole.ID
			// Update permissions
			if err := model.DB.Model(&existingRole).Updates(map[string]interface{}{
				"description": r.Description,
				"permissions": r.Permissions,
				"is_system":   r.IsSystem,
			}).Error; err != nil {
				logger.Error("failed to update role", zap.String("role", r.Name), zap.Error(err))
			} else {
				logger.Info("role updated", zap.String("role", r.Name))
			}
		}
	}

	// Default password for all users
	defaultPassword := "admin123"
	hashedPassword, err := crypto.HashPassword(defaultPassword)
	if err != nil {
		logger.Fatal("failed to hash password", zap.Error(err))
	}

	// Define users
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

	// Create or update users
	for _, u := range users {
		roleID, ok := roleMap[u.RoleName]
		if !ok {
			logger.Error("role not found for user", zap.String("user", u.Username), zap.String("role", u.RoleName))
			continue
		}

		var existingUser model.User
		result := model.DB.Where("username = ?", u.Username).First(&existingUser)
		if result.Error != nil {
			// User doesn't exist, create it
			newUser := model.User{
				Username: u.Username,
				Email:    u.Email,
				Password: hashedPassword,
				RealName: u.RealName,
				Status:   1,
				RoleID:   roleID,
			}
			if err := model.DB.Create(&newUser).Error; err != nil {
				logger.Error("failed to create user", zap.String("user", u.Username), zap.Error(err))
			} else {
				logger.Info("user created", zap.String("user", u.Username), zap.String("role", u.RoleName))
			}
		} else {
			// User exists, update role if needed
			if existingUser.RoleID != roleID {
				model.DB.Model(&existingUser).Update("role_id", roleID)
				logger.Info("user role updated", zap.String("user", u.Username), zap.String("role", u.RoleName))
			}
		}
	}

	fmt.Println("=== Initialization Complete ===")
	fmt.Println("")
	fmt.Println("Default users (password: admin123):")
	fmt.Println("  - admin     : 系统管理员")
	fmt.Println("  - operator  : 运维工程师")
	fmt.Println("  - developer : 开发人员")
	fmt.Println("  - viewer    : 只读用户")
	fmt.Println("")
	fmt.Println("⚠️  Please change the default passwords after first login!")
}
