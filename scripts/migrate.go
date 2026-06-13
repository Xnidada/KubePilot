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

	// Auto migrate
	if err := model.AutoMigrate(); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}

	logger.Info("database migration completed")
}
