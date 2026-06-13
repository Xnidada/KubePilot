package router

import (
	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/handler/alert"
	"github.com/kubepilot/kubepilot/internal/handler/auth"
	"github.com/kubepilot/kubepilot/internal/handler/cluster"
	"github.com/kubepilot/kubepilot/internal/handler/system"
	"github.com/kubepilot/kubepilot/internal/handler/workload"
	"github.com/kubepilot/kubepilot/internal/middleware"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	authService "github.com/kubepilot/kubepilot/internal/service/auth"
	clusterService "github.com/kubepilot/kubepilot/internal/service/cluster"
)

func Setup(cfg *config.Config) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()

	// Global middleware
	r.Use(middleware.CORSMiddleware())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.AuditMiddleware()) // 启用审计日志

	// Initialize JWT manager
	jwtManager := utils.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireTime, cfg.JWT.Issuer)

	// Initialize services
	authSvc := authService.NewService(model.DB, jwtManager)
	clusterSvc := clusterService.NewService(model.DB, cfg.JWT.Secret)

	// Initialize handlers
	authHandler := auth.NewHandler(authSvc)
	clusterHandler := cluster.NewHandler(clusterSvc)
	workloadHandler := workload.NewHandler()
	systemHandler := system.NewHandler(model.DB)
	alertHandler := alert.NewHandler(model.DB)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Auth routes (public)
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/register", authHandler.Register)
		}

		// Protected routes
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware(jwtManager))
		protected.Use(middleware.AutoRBACMiddleware()) // 启用RBAC权限检查
		{
			// User profile
			protected.GET("/profile", authHandler.GetProfile)
			protected.PUT("/profile/password", authHandler.ChangePassword)

			// System management (Admin only)
			systemGroup := protected.Group("/system")
			{
				// User management
				systemGroup.GET("/users", systemHandler.ListUsers)
				systemGroup.POST("/users", systemHandler.CreateUser)
				systemGroup.GET("/users/:id", systemHandler.GetUser)
				systemGroup.PUT("/users/:id", systemHandler.UpdateUser)
				systemGroup.DELETE("/users/:id", systemHandler.DeleteUser)
				systemGroup.POST("/users/:id/reset-password", systemHandler.ResetPassword)

				// Role management
				systemGroup.GET("/roles", systemHandler.ListRoles)
				systemGroup.POST("/roles", systemHandler.CreateRole)
				systemGroup.GET("/roles/:id", systemHandler.GetRole)
				systemGroup.PUT("/roles/:id", systemHandler.UpdateRole)
				systemGroup.DELETE("/roles/:id", systemHandler.DeleteRole)

				// Permission resources
				systemGroup.GET("/resources", systemHandler.GetResourceTypes)
				systemGroup.GET("/actions", systemHandler.GetActionTypes)
				systemGroup.GET("/role-templates", systemHandler.GetRoleTemplates)

				// Audit logs
				systemGroup.GET("/audit-logs", systemHandler.GetAuditLogs)
			}

			// Alert management
			alertGroup := protected.Group("/alerts")
			{
				// Alert rules
				alertGroup.GET("/rules", alertHandler.ListAlertRules)
				alertGroup.POST("/rules", alertHandler.CreateAlertRule)
				alertGroup.PUT("/rules/:id", alertHandler.UpdateAlertRule)
				alertGroup.DELETE("/rules/:id", alertHandler.DeleteAlertRule)

				// Alert history
				alertGroup.GET("/history", alertHandler.ListAlertHistory)

				// Notification channels
				alertGroup.GET("/channels", alertHandler.ListNotificationChannels)
				alertGroup.POST("/channels", alertHandler.CreateNotificationChannel)
				alertGroup.PUT("/channels/:id", alertHandler.UpdateNotificationChannel)
				alertGroup.DELETE("/channels/:id", alertHandler.DeleteNotificationChannel)
			}

			// Cluster management
			clusters := protected.Group("/clusters")
			{
				clusters.GET("", clusterHandler.List)
				clusters.POST("", clusterHandler.Create)
				clusters.GET("/:id", clusterHandler.Get)
				clusters.PUT("/:id", clusterHandler.Update)
				clusters.DELETE("/:id", clusterHandler.Delete)
				clusters.POST("/:id/health", clusterHandler.HealthCheck)
				clusters.GET("/:id/info", clusterHandler.GetClusterInfo)
				clusters.GET("/:id/namespaces", clusterHandler.GetNamespaces)
				clusters.GET("/:id/nodes", clusterHandler.GetNodes)
			}

			// Workload management
			workloads := protected.Group("/clusters/:id/workloads")
			{
				// Deployments
				workloads.GET("/deployments", workloadHandler.ListDeployments)
				workloads.POST("/deployments", workloadHandler.CreateDeployment)
				workloads.POST("/deployments/enterprise", workloadHandler.CreateEnterpriseDeployment)
				workloads.GET("/deployments/:ns/:name", workloadHandler.GetDeployment)
				workloads.PUT("/deployments/:ns/:name", workloadHandler.UpdateDeployment)
				workloads.GET("/deployments/:ns/:name/services", workloadHandler.GetDeploymentServices)
				workloads.GET("/deployments/:ns/:name/history", workloadHandler.GetDeploymentHistory)
				workloads.POST("/deployments/:ns/:name/rollback", workloadHandler.RollbackDeployment)
				workloads.POST("/deployments/:ns/:name/scale", workloadHandler.ScaleDeployment)
				workloads.DELETE("/deployments/:ns/:name", workloadHandler.DeleteDeployment)

				// Pods
				workloads.GET("/pods", workloadHandler.ListPods)
				workloads.POST("/pods", workloadHandler.CreatePod)
				workloads.GET("/pods/:ns/:name", workloadHandler.GetPod)
				workloads.GET("/pods/:ns/:name/logs", workloadHandler.GetPodLogs)
				workloads.GET("/pods/:ns/:name/containers", workloadHandler.GetPodContainers)
				workloads.DELETE("/pods/:ns/:name", workloadHandler.DeletePod)

				// Services
				workloads.GET("/services", workloadHandler.ListServices)
				workloads.POST("/services", workloadHandler.CreateService)
				workloads.GET("/services/:ns/:name", workloadHandler.GetService)
				workloads.PUT("/services/:ns/:name", workloadHandler.UpdateService)
				workloads.DELETE("/services/:ns/:name", workloadHandler.DeleteService)

				// Nodes
				workloads.GET("/nodes", workloadHandler.ListNodes)

				// Namespaces
				workloads.GET("/namespaces", workloadHandler.ListNamespaces)

				// Events
				workloads.GET("/events", workloadHandler.ListEvents)

				// PV
				workloads.GET("/pvs", workloadHandler.ListPVs)
				workloads.POST("/pvs", workloadHandler.CreatePV)
				workloads.GET("/pvs/:name", workloadHandler.GetPV)
				workloads.DELETE("/pvs/:name", workloadHandler.DeletePV)

				// PVC
				workloads.GET("/pvcs", workloadHandler.ListPVCs)
				workloads.POST("/pvcs", workloadHandler.CreatePVC)
				workloads.GET("/pvcs/:ns/:name", workloadHandler.GetPVC)
				workloads.DELETE("/pvcs/:ns/:name", workloadHandler.DeletePVC)

				// StorageClass
				workloads.GET("/storageclasses", workloadHandler.ListStorageClasses)

				// Metrics
				workloads.GET("/metrics/pods", workloadHandler.GetPodMetrics)
				workloads.GET("/metrics/deployments", workloadHandler.GetDeploymentMetrics)
				workloads.GET("/metrics/nodes", workloadHandler.GetNodeMetrics)
				workloads.GET("/metrics/overview", workloadHandler.GetClusterOverview)
			}
		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// WebSocket terminal (需要认证)
	r.GET("/api/v1/ws/terminal/:id/:ns/:name", workloadHandler.PodTerminal)

	// Serve frontend static files
	r.Static("/assets", "./web/assets")
	r.StaticFile("/vite.svg", "./web/vite.svg")

	// SPA fallback - serve index.html for all non-API routes
	r.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(404, gin.H{"code": 404, "message": "API not found"})
			return
		}
		c.File("./web/index.html")
	})

	return r
}
