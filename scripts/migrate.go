//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"github.com/kubepilot/kubepilot/internal/modules"
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

	if err := model.InitDatabase(
		cfg.Database.Driver,
		cfg.Database.DSN(),
		cfg.Database.MaxIdleConns,
		cfg.Database.MaxOpenConns,
	); err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}

	modReg := module.NewRegistry(cfg.ModuleEnabled, logger.GetLogger())
	modules.RegisterAll(modReg)

	if err := model.AutoMigrateCore(); err != nil {
		logger.Fatal("failed to migrate core database", zap.Error(err))
	}
	if err := modReg.Migrate(model.DB); err != nil {
		logger.Fatal("failed to migrate module database", zap.Error(err))
	}

	logger.Info("database migration completed")
}
