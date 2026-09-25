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
	if err := model.AutoMigrateCore(); err != nil {
		logger.Fatal("failed to migrate core tables", zap.Error(err))
	}

	if err := model.SeedData(); err != nil {
		logger.Fatal("failed to seed data", zap.Error(err))
	}

	fmt.Println("=== Initialization Complete ===")
	fmt.Println("Existing users, roles and cluster grants were preserved.")
	if os.Getenv("KUBEPILOT_SEED_DEMO_USERS") == "true" {
		fmt.Println("Demo users were created only if absent; grant cluster access explicitly.")
	}
}
