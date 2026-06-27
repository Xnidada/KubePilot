package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/handler/workload"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"github.com/kubepilot/kubepilot/internal/router"
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
	defer logger.Sync()

	logger.Info("starting KubePilot server...")

	// Initialize database
	driver := cfg.Database.Driver
	if driver == "" {
		driver = "postgres"
	}
	if err := model.InitDatabase(driver, cfg.Database.DSN(), cfg.Database.MaxIdleConns, cfg.Database.MaxOpenConns); err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	logger.Info("database connected", zap.String("driver", driver))

	// Auto migrate
	if err := model.AutoMigrate(); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}
	logger.Info("database migrated")

	// Seed default data (roles, permissions, users)
	if err := model.SeedData(); err != nil {
		logger.Warn("failed to seed data", zap.Error(err))
	}
	logger.Info("seed data initialized")

	// Initialize cache
	cacheInstance := cache.New(cache.Config{
		Type:     cfg.Cache.Type,
		Addr:     cfg.Cache.Addr,
		Password: cfg.Cache.Password,
		DB:       cfg.Cache.DB,
	})
	defer cacheInstance.Close()
	logger.Info("cache initialized", zap.String("type", cfg.Cache.Type))

	// Initialize K8S client manager with database adapter
	dbAdapter := k8s.NewClusterDBAdapter(cfg.JWT.Secret)
	k8s.InitClientManager(cfg.K8S.QPS, cfg.K8S.Burst, dbAdapter)
	logger.Info("K8S client manager initialized")

	// 启动 node-shell Pod 清理协程（每 30 分钟清理一次，删除 1 小时未使用的 Pod）
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			// 遍历所有集群进行清理
			for _, clusterID := range k8s.Manager.ListClusters() {
				client, err := k8s.Manager.GetClient(clusterID)
				if err != nil {
					continue
				}
				workload.CleanupNodeShellPods(client, 1*time.Hour)
			}
		}
	}()

	// Setup router
	r := router.Setup(cfg, cacheInstance)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed to start", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	// Close database
	if err := model.Close(); err != nil {
		logger.Error("failed to close database", zap.Error(err))
	}

	logger.Info("server exited")
}
