package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/handler/alert"
	aiopsHandler "github.com/kubepilot/kubepilot/internal/handler/aiops"
	"github.com/kubepilot/kubepilot/internal/handler/auth"
	"github.com/kubepilot/kubepilot/internal/handler/cluster"
	"github.com/kubepilot/kubepilot/internal/handler/system"
	"github.com/kubepilot/kubepilot/internal/handler/workload"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/llm"
	"github.com/kubepilot/kubepilot/internal/middleware"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	aiopsService "github.com/kubepilot/kubepilot/internal/service/aiops"
	authService "github.com/kubepilot/kubepilot/internal/service/auth"
	clusterService "github.com/kubepilot/kubepilot/internal/service/cluster"
	"go.uber.org/zap"
)

func Setup(cfg *config.Config, cacheInstance cache.Cache) *gin.Engine {
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

	// Initialize AIOps service
	llmConfig := &llm.LLMConfig{
		Provider:    llm.LLMProvider(cfg.LLM.Provider),
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		Model:       cfg.LLM.Model,
		Temperature: cfg.LLM.Temperature,
		MaxTokens:   cfg.LLM.MaxTokens,
		Timeout:     cfg.LLM.Timeout,
	}
	aiopsSvc, err := aiopsService.NewService(model.DB, llmConfig, cfg.JWT.Secret, cacheInstance)
	if err != nil {
		// Log warning but continue - AI features will be unavailable
		logger.Warn("failed to initialize AIOps service", zap.Error(err))
	}

	// Initialize handlers
	authHandler := auth.NewHandler(authSvc, model.DB)
	twoFactorHandler := auth.NewTwoFactorHandler(model.DB)
	clusterHandler := cluster.NewHandler(clusterSvc)
	workloadHandler := workload.NewHandler()
	workloadHandler.SetKubectlExecutor(k8s.NewKubectlExecutor(cfg.JWT.Secret))
	systemHandler := system.NewHandler(model.DB)
	alertHandler := alert.NewHandler(model.DB)
	aiopsHandler := aiopsHandler.NewHandler(aiopsSvc, model.DB)
	inspectionHandler := NewInspectionHandler(model.DB)
	eventForwardHandler := NewEventForwardHandler(model.DB)
	oauthHandler := NewOAuthHandler(model.DB, authSvc, cacheInstance)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Auth routes (public) - with rate limiting
		authGroup := v1.Group("/auth")
		authGroup.Use(middleware.RateLimitMiddleware(10, time.Minute, cacheInstance)) // 10 requests per minute
		{
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/2fa/verify", twoFactorHandler.LoginVerify) // 2FA 登录验证
		}

		// OAuth routes (public)
		oauthGroup := v1.Group("/oauth")
		{
			oauthGroup.GET("/providers", oauthHandler.ListProviders)
			oauthGroup.GET("/:provider/login", oauthHandler.Login)
			oauthGroup.GET("/:provider/callback", oauthHandler.Callback)
		}

		// Protected routes
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware(jwtManager))
		protected.Use(middleware.AutoRBACMiddleware()) // 启用RBAC权限检查
		{
			// User profile
			protected.GET("/profile", authHandler.GetProfile)
			protected.PUT("/profile/password", authHandler.ChangePassword)

			// Two-Factor Authentication
			twoFactorGroup := protected.Group("/2fa")
			{
				twoFactorGroup.GET("/status", twoFactorHandler.Status)
				twoFactorGroup.POST("/setup", twoFactorHandler.Setup)
				twoFactorGroup.POST("/verify-enable", twoFactorHandler.VerifyAndEnable)
				twoFactorGroup.POST("/disable", twoFactorHandler.Disable)
			}

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
				workloads.GET("/nodes/:name", workloadHandler.GetNode)
				workloads.PUT("/nodes/:name", workloadHandler.UpdateNode)

				// Namespaces
				workloads.GET("/namespaces", workloadHandler.ListNamespaces)
				workloads.GET("/namespaces/names", workloadHandler.ListNamespaceNames)
				workloads.POST("/namespaces", workloadHandler.CreateNamespace)
				workloads.GET("/namespaces/:name", workloadHandler.GetNamespaceDetail)
				workloads.PUT("/namespaces/:name", workloadHandler.UpdateNamespace)
				workloads.DELETE("/namespaces/:name", workloadHandler.DeleteNamespace)
				workloads.GET("/namespaces/:name/quotas", workloadHandler.GetResourceQuota)
				workloads.POST("/namespaces/:name/quotas", workloadHandler.CreateResourceQuota)
				workloads.PUT("/namespaces/:name/quotas", workloadHandler.UpdateResourceQuota)
				workloads.DELETE("/namespaces/:name/quotas", workloadHandler.DeleteResourceQuota)

				// Events
				workloads.GET("/events", workloadHandler.ListEvents)

				// ConfigMaps
				workloads.GET("/configmaps", workloadHandler.ListConfigMaps)
				workloads.POST("/configmaps", workloadHandler.CreateConfigMap)
				workloads.GET("/configmaps/:ns/:name", workloadHandler.GetConfigMap)
				workloads.PUT("/configmaps/:ns/:name", workloadHandler.UpdateConfigMap)
				workloads.DELETE("/configmaps/:ns/:name", workloadHandler.DeleteConfigMap)

				// Secrets
				workloads.GET("/secrets", workloadHandler.ListSecrets)
				workloads.POST("/secrets", workloadHandler.CreateSecret)
				workloads.GET("/secrets/:ns/:name", workloadHandler.GetSecret)
				workloads.PUT("/secrets/:ns/:name", workloadHandler.UpdateSecret)
				workloads.DELETE("/secrets/:ns/:name", workloadHandler.DeleteSecret)

				// Ingresses
				workloads.GET("/ingresses", workloadHandler.ListIngresses)
				workloads.POST("/ingresses", workloadHandler.CreateIngress)
				workloads.GET("/ingresses/:ns/:name", workloadHandler.GetIngress)
				workloads.PUT("/ingresses/:ns/:name", workloadHandler.UpdateIngress)
				workloads.DELETE("/ingresses/:ns/:name", workloadHandler.DeleteIngress)

				// PV
				workloads.GET("/pvs", workloadHandler.ListPVs)
				workloads.POST("/pvs", workloadHandler.CreatePV)
				workloads.GET("/pvs/:name", workloadHandler.GetPV)
				workloads.PUT("/pvs/:name", workloadHandler.UpdatePV)
				workloads.DELETE("/pvs/:name", workloadHandler.DeletePV)

				// PVC
				workloads.GET("/pvcs", workloadHandler.ListPVCs)
				workloads.POST("/pvcs", workloadHandler.CreatePVC)
				workloads.GET("/pvcs/:ns/:name", workloadHandler.GetPVC)
				workloads.PUT("/pvcs/:ns/:name", workloadHandler.UpdatePVC)
				workloads.DELETE("/pvcs/:ns/:name", workloadHandler.DeletePVC)

				// StorageClass
				workloads.GET("/storageclasses", workloadHandler.ListStorageClasses)
				workloads.POST("/storageclasses", workloadHandler.CreateStorageClass)
				workloads.GET("/storageclasses/:name", workloadHandler.GetStorageClass)
				workloads.PUT("/storageclasses/:name", workloadHandler.UpdateStorageClass)
				workloads.DELETE("/storageclasses/:name", workloadHandler.DeleteStorageClass)

				// Metrics
				workloads.GET("/metrics/pods", workloadHandler.GetPodMetrics)
				workloads.GET("/metrics/deployments", workloadHandler.GetDeploymentMetrics)
				workloads.GET("/metrics/nodes", workloadHandler.GetNodeMetrics)
				workloads.GET("/metrics/overview", workloadHandler.GetClusterOverview)

				// StatefulSets
				workloads.GET("/statefulsets", workloadHandler.ListStatefulSets)
				workloads.POST("/statefulsets", workloadHandler.CreateStatefulSet)
				workloads.GET("/statefulsets/:ns/:name", workloadHandler.GetStatefulSet)
				workloads.PUT("/statefulsets/:ns/:name", workloadHandler.UpdateStatefulSet)
				workloads.DELETE("/statefulsets/:ns/:name", workloadHandler.DeleteStatefulSet)

				// DaemonSets
				workloads.GET("/daemonsets", workloadHandler.ListDaemonSets)
				workloads.POST("/daemonsets", workloadHandler.CreateDaemonSet)
				workloads.GET("/daemonsets/:ns/:name", workloadHandler.GetDaemonSet)
				workloads.PUT("/daemonsets/:ns/:name", workloadHandler.UpdateDaemonSet)
				workloads.DELETE("/daemonsets/:ns/:name", workloadHandler.DeleteDaemonSet)

				// Jobs
				workloads.GET("/jobs", workloadHandler.ListJobs)
				workloads.POST("/jobs", workloadHandler.CreateJob)
				workloads.GET("/jobs/:ns/:name", workloadHandler.GetJob)
				workloads.DELETE("/jobs/:ns/:name", workloadHandler.DeleteJob)

				// CronJobs
				workloads.GET("/cronjobs", workloadHandler.ListCronJobs)
				workloads.POST("/cronjobs", workloadHandler.CreateCronJob)
				workloads.GET("/cronjobs/:ns/:name", workloadHandler.GetCronJob)
				workloads.PUT("/cronjobs/:ns/:name", workloadHandler.UpdateCronJob)
				workloads.DELETE("/cronjobs/:ns/:name", workloadHandler.DeleteCronJob)

				// ReplicaSets
				workloads.GET("/replicasets", workloadHandler.ListReplicaSets)
				workloads.GET("/replicasets/:ns/:name", workloadHandler.GetReplicaSet)
				workloads.POST("/replicasets/:ns/:name/scale", workloadHandler.ScaleReplicaSet)
				workloads.DELETE("/replicasets/:ns/:name", workloadHandler.DeleteReplicaSet)

				// Pods (Update not supported - Pod is immutable)
				workloads.PUT("/pods/:ns/:name", workloadHandler.UpdatePod)

				// CRDs
				workloads.GET("/crds", workloadHandler.ListCRDs)

				// Custom Resources (CRD instances)
				workloads.GET("/crds/:group/:version/:resource", workloadHandler.ListCustomResources)
				workloads.POST("/crds/:group/:version/:resource", workloadHandler.CreateCustomResource)
				workloads.GET("/crds/:group/:version/:resource/:name", workloadHandler.GetCustomResource)
				workloads.PUT("/crds/:group/:version/:resource/:name", workloadHandler.UpdateCustomResource)
				workloads.DELETE("/crds/:group/:version/:resource/:name", workloadHandler.DeleteCustomResource)

				// Cluster Events
				workloads.GET("/cluster-events", workloadHandler.ListClusterEvents)

				// Pod Files
				workloads.GET("/pods/:ns/:name/files", workloadHandler.ListPodFiles)
				workloads.GET("/pods/:ns/:name/files/read", workloadHandler.ReadPodFile)
				workloads.POST("/pods/:ns/:name/files/write", workloadHandler.WritePodFile)
				workloads.DELETE("/pods/:ns/:name/files", workloadHandler.DeletePodFile)
				workloads.GET("/pods/:ns/:name/files/download", workloadHandler.DownloadPodFile)

				// YAML Operations
				workloads.GET("/yaml/:type/:ns/:name", workloadHandler.GetResourceYAML)
				workloads.POST("/yaml/apply", workloadHandler.ApplyResourceYAML)
				workloads.POST("/yaml/delete", workloadHandler.DeleteResourceYAML)

				// Resource Events & Describe
				workloads.GET("/events/:type/:ns/:name", workloadHandler.GetResourceEvents)
				workloads.GET("/describe/:type/:ns/:name", workloadHandler.DescribeResource)
			}

			// AIOps routes
			aiopsGroup := protected.Group("/aiops")
			{
				// LLM Config
				aiopsGroup.GET("/configs", aiopsHandler.ListLLMConfigs)
				aiopsGroup.POST("/configs", aiopsHandler.SaveLLMConfig)
				aiopsGroup.GET("/configs/default", aiopsHandler.GetLLMConfig)
				aiopsGroup.GET("/configs/:id", aiopsHandler.GetLLMConfigByID)
				aiopsGroup.PUT("/configs/:id", aiopsHandler.UpdateLLMConfig)
				aiopsGroup.DELETE("/configs/:id", aiopsHandler.DeleteLLMConfig)
				aiopsGroup.POST("/configs/:id/set-default", aiopsHandler.SetDefaultLLMConfig)
				aiopsGroup.POST("/configs/test", aiopsHandler.TestLLMConfig)

				// Conversations
				aiopsGroup.GET("/conversations", aiopsHandler.ListConversations)
				aiopsGroup.POST("/conversations", aiopsHandler.CreateConversation)
				aiopsGroup.GET("/conversations/:id", aiopsHandler.GetConversation)
				aiopsGroup.PUT("/conversations/:id", aiopsHandler.UpdateConversation)
				aiopsGroup.DELETE("/conversations/:id", aiopsHandler.DeleteConversation)
				aiopsGroup.POST("/conversations/:id/clear", aiopsHandler.ClearConversation)

				// Messages
				aiopsGroup.GET("/conversations/:id/messages", aiopsHandler.ListMessages)
				aiopsGroup.POST("/conversations/:id/messages", aiopsHandler.AddMessage)
				aiopsGroup.DELETE("/conversations/:id/messages/:msgId", aiopsHandler.DeleteMessage)

				// Chat
				aiopsGroup.POST("/chat", aiopsHandler.Chat)
				aiopsGroup.POST("/chat/stream", aiopsHandler.ChatStream)

				// AI 驱动功能
				aiopsGroup.POST("/explain", aiopsHandler.ExplainText)
				aiopsGroup.POST("/explain/stream", aiopsHandler.ExplainTextStream)
				aiopsGroup.POST("/resource-guide", aiopsHandler.GetResourceGuide)
				aiopsGroup.POST("/translate-yaml", aiopsHandler.TranslateYAML)
				aiopsGroup.POST("/analyze-describe", aiopsHandler.AnalyzeDescribe)
				aiopsGroup.POST("/analyze-logs", aiopsHandler.AnalyzeLogs)

				// Diagnosis
				aiopsGroup.POST("/diagnose", aiopsHandler.Diagnose)

				// Agent
				aiopsGroup.POST("/agent", aiopsHandler.AgentChat)
				aiopsGroup.POST("/agent/confirm/:actionId", aiopsHandler.AgentConfirmAction)
				aiopsGroup.POST("/agent/execute", aiopsHandler.AgentExecute)

				// Kubectl
				aiopsGroup.POST("/kubectl", aiopsHandler.KubectlExecute)
				aiopsGroup.GET("/kubectl/:id/query", aiopsHandler.KubectlQuery)
			}

			// Inspection routes
			inspectionGroup := protected.Group("/inspection")
			{
				inspectionGroup.GET("/rules", inspectionHandler.ListRules)
				inspectionGroup.POST("/rules", inspectionHandler.CreateRule)
				inspectionGroup.GET("/rules/:id", inspectionHandler.GetRule)
				inspectionGroup.PUT("/rules/:id", inspectionHandler.UpdateRule)
				inspectionGroup.DELETE("/rules/:id", inspectionHandler.DeleteRule)
				inspectionGroup.POST("/rules/:id/run", inspectionHandler.RunInspection)
				inspectionGroup.GET("/reports", inspectionHandler.ListReports)
				inspectionGroup.GET("/reports/:id", inspectionHandler.GetReport)
				inspectionGroup.GET("/reports/:id/results", inspectionHandler.GetReportResults)
			}

			// Event Forward routes
			eventForwardGroup := protected.Group("/event-forward")
			{
				eventForwardGroup.GET("/rules", eventForwardHandler.ListRules)
				eventForwardGroup.POST("/rules", eventForwardHandler.CreateRule)
				eventForwardGroup.GET("/rules/:id", eventForwardHandler.GetRule)
				eventForwardGroup.PUT("/rules/:id", eventForwardHandler.UpdateRule)
				eventForwardGroup.DELETE("/rules/:id", eventForwardHandler.DeleteRule)
				eventForwardGroup.POST("/rules/:id/test", eventForwardHandler.TestRule)
				eventForwardGroup.GET("/logs", eventForwardHandler.ListLogs)
			}
		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// WebSocket terminal (需要认证)
	r.GET("/api/v1/ws/terminal/:id/:ns/:name", workloadHandler.PodTerminal)
	r.GET("/api/v1/ws/node-shell/:id/:name", workloadHandler.NodeShell)

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
