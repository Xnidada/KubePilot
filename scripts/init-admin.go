//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output); err != nil {
		fmt.Printf("failed to init logger: %v\n", err)
		os.Exit(1)
	}

	if err := model.InitDatabase(cfg.Database.Driver, cfg.Database.DSN(), cfg.Database.MaxIdleConns, cfg.Database.MaxOpenConns); err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}

	if err := model.SeedData(); err != nil {
		logger.Fatal("failed to seed data", zap.Error(err))
	}

	fmt.Println("=== Initialization Complete ===")
	fmt.Println("")
	fmt.Println("Default users (password: admin123):")
	fmt.Println("  - admin     : 系统管理员")
	fmt.Println("  - operator  : 运维工程师")
	fmt.Println("  - developer : 开发人员")
	fmt.Println("  - viewer    : 只读用户（不含 AI 智能）")
	fmt.Println("  - aiviewer  : AI 只读用户（可浏览 AI 智能，不可执行）")
	fmt.Println("")
	fmt.Println("⚠️  Please change the default passwords after first login!")
}
