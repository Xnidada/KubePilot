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
	if err := model.InitDatabase(cfg.Database.DSN()); err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}

	// Create default roles with proper permissions
	roles := []model.Role{
		{Name: "admin", Description: "System Administrator", Permissions: model.RoleTemplates["admin"].ToJSON(), IsSystem: true},
		{Name: "operator", Description: "Operator", Permissions: model.RoleTemplates["operator"].ToJSON(), IsSystem: false},
		{Name: "user", Description: "Regular User", Permissions: model.RoleTemplates["user"].ToJSON(), IsSystem: false},
		{Name: "viewer", Description: "Viewer", Permissions: model.RoleTemplates["viewer"].ToJSON(), IsSystem: false},
	}

	for _, role := range roles {
		var existingRole model.Role
		result := model.DB.Where("name = ?", role.Name).First(&existingRole)
		if result.Error != nil {
			// Role doesn't exist, create it
			if err := model.DB.Create(&role).Error; err != nil {
				logger.Error("failed to create role", zap.String("role", role.Name), zap.Error(err))
			} else {
				logger.Info("role created", zap.String("role", role.Name))
			}
		} else {
			// Role exists, update permissions if empty
			if existingRole.Permissions == "" || existingRole.Permissions == "{}" {
				if err := model.DB.Model(&existingRole).Update("permissions", role.Permissions).Error; err != nil {
					logger.Error("failed to update role permissions", zap.String("role", role.Name), zap.Error(err))
				} else {
					logger.Info("role permissions updated", zap.String("role", role.Name))
				}
			}
		}
	}

	// Create admin user
	var adminRole model.Role
	if err := model.DB.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
		logger.Fatal("admin role not found", zap.Error(err))
	}

	var adminUser model.User
	result := model.DB.Where("username = ?", "admin").First(&adminUser)
	if result.Error != nil {
		// User doesn't exist, create it
		hashedPassword, err := crypto.HashPassword("admin123")
		if err != nil {
			logger.Fatal("failed to hash password", zap.Error(err))
		}

		adminUser = model.User{
			Username: "admin",
			Email:    "admin@kubepilot.io",
			Password: hashedPassword,
			RealName: "Administrator",
			Status:   1,
			RoleID:   adminRole.ID,
		}

		if err := model.DB.Create(&adminUser).Error; err != nil {
			logger.Fatal("failed to create admin user", zap.Error(err))
		}
		logger.Info("admin user created successfully")
	} else {
		logger.Info("admin user already exists")
	}

	fmt.Println("=== Initialization Complete ===")
	fmt.Println("Username: admin")
	fmt.Println("Password: admin123")
}
