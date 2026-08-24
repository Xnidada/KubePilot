package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/handler/alert"
	"github.com/kubepilot/kubepilot/internal/handler/auth"
	"github.com/kubepilot/kubepilot/internal/handler/cluster"
	opsHandler "github.com/kubepilot/kubepilot/internal/handler/ops"
	"github.com/kubepilot/kubepilot/internal/handler/system"
	"github.com/kubepilot/kubepilot/internal/handler/tenant"
	"github.com/kubepilot/kubepilot/internal/handler/workload"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/middleware"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	"github.com/kubepilot/kubepilot/internal/pkg/wsticket"
	authService "github.com/kubepilot/kubepilot/internal/service/auth"
	clusterService "github.com/kubepilot/kubepilot/internal/service/cluster"
	"go.uber.org/zap"
)

func Setup(cfg *config.Config, cacheInstance cache.Cache, modReg *module.Registry) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()

	// Trust internal network proxies (K8s pod/service/ingress networks)
	// so ClientIP() correctly extracts the real client IP from X-Forwarded-For
	_ = r.SetTrustedProxies([]string{
		"10.0.0.0/8",     // K8s pod network (common)
		"172.16.0.0/12",  // K8s pod network (common) + Docker default
		"192.168.0.0/16", // Local network
		"127.0.0.0/8",    // Loopback
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 private (K8s dual-stack)
	})

	// Global middleware
	r.Use(middleware.CORSMiddleware())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.AuditMiddleware()) // 启用审计日志

	// Initialize JWT manager
	jwtManager := utils.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireTime, cfg.JWT.Issuer)
	webSocketTicketManager := wsticket.NewManager(cacheInstance)

	var policyExtra func(*authz.Registry) error
	if modReg != nil {
		policyExtra = modReg.RegisterPolicies
	}
	_, authorizer := SetupAuthorizer(model.DB, policyExtra)

	// Initialize core services / handlers
	encryptKey := cfg.EncryptKey()
	authSvc := authService.NewService(model.DB, jwtManager)
	clusterSvc := clusterService.NewService(model.DB, encryptKey)

	authHandler := auth.NewHandler(authSvc, model.DB, cacheInstance)
	webSocketTicketHandler := auth.NewWebSocketTicketHandler(webSocketTicketManager)
	twoFactorHandler := auth.NewTwoFactorHandler(model.DB, authSvc, cacheInstance)
	clusterHandler := cluster.NewHandler(clusterSvc)
	workloadHandler := workload.NewHandler()
	workloadHandler.SetKubectlExecutor(k8s.NewKubectlExecutor(encryptKey))
	systemHandler := system.NewHandler(model.DB)
	alertHandler := alert.NewHandler(model.DB)
	oauthHandler := NewOAuthHandler(model.DB, authSvc, cacheInstance)
	opsHandler := opsHandler.NewHandler()
	tenantHandler := tenant.NewHandler(model.DB)

	host := &module.Host{
		DB:         model.DB,
		Config:     cfg,
		Cache:      cacheInstance,
		EncryptKey: encryptKey,
		Logger:     logger.GetLogger(),
	}
	modCtx := &module.Context{Host: host}

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
		protected.Use(func(c *gin.Context) {
			c.Set(authz.ContextAuthorizerKey, authorizer)
			c.Set("authz_grant_resolver", authorizer)
			c.Next()
		})
		protected.Use(middleware.PolicyAuthzMiddleware(authorizer))
		{
			// User profile
			protected.GET("/profile", authHandler.GetProfile)
			protected.PUT("/profile/password", authHandler.ChangePassword)

			if modReg != nil {
				protected.GET("/modules", func(c *gin.Context) {
					response.Success(c, modReg.Status(c.Request.Context()))
				})
				protected.GET("/modules/menus", func(c *gin.Context) {
					response.Success(c, modReg.Menus())
				})
				protected.GET("/modules/:name", func(c *gin.Context) {
					st, ok := modReg.StatusOne(c.Request.Context(), c.Param("name"))
					if !ok {
						response.NotFound(c, "module not found")
						return
					}
					response.Success(c, st)
				})
			}

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
				systemGroup.GET(
					"/users/:id/clusters",
					middleware.RequirePermission("users", "admin"),
					middleware.RBACMiddleware(),
					systemHandler.ListUserClusters,
				)
				systemGroup.PUT(
					"/users/:id/clusters",
					middleware.RequirePermission("users", "admin"),
					middleware.RBACMiddleware(),
					systemHandler.ReplaceUserClusters,
				)

				// User groups
				systemGroup.GET("/user-groups", systemHandler.ListUserGroups)
				systemGroup.POST("/user-groups", systemHandler.CreateUserGroup)
				systemGroup.GET("/user-groups/:id", systemHandler.GetUserGroup)
				systemGroup.PUT("/user-groups/:id", systemHandler.UpdateUserGroup)
				systemGroup.DELETE("/user-groups/:id", systemHandler.DeleteUserGroup)
				systemGroup.GET("/user-groups/:id/members", systemHandler.ListUserGroupMembers)
				systemGroup.PUT("/user-groups/:id/members", systemHandler.ReplaceUserGroupMembers)
				systemGroup.GET("/user-groups/:id/clusters", systemHandler.ListUserGroupClusters)
				systemGroup.PUT("/user-groups/:id/clusters", systemHandler.ReplaceUserGroupClusters)
				systemGroup.GET("/users/:id/effective-cluster-permissions", systemHandler.GetUserEffectiveClusterPermissions)
				systemGroup.GET("/users/:id/effective-access", systemHandler.GetUserEffectiveClusterPermissions)

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

				// Login logs
				systemGroup.GET("/login-logs", systemHandler.GetLoginLogs)
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

				// NetworkPolicy
				workloads.GET("/networkpolicies", workloadHandler.ListNetworkPolicies)
				workloads.POST("/networkpolicies", workloadHandler.CreateNetworkPolicy)
				workloads.GET("/networkpolicies/:ns/:name", workloadHandler.GetNetworkPolicy)
				workloads.DELETE("/networkpolicies/:ns/:name", workloadHandler.DeleteNetworkPolicy)

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

				// HPA 管理
				workloads.GET("/hpas", workloadHandler.ListHPAs)
				workloads.POST("/hpas", workloadHandler.CreateHPA)
				workloads.DELETE("/hpas/:ns/:name", workloadHandler.DeleteHPA)

				// 批量操作
				workloads.POST("/batch", workloadHandler.BatchOperation)

				// Pod 亲和性
				workloads.GET("/deployments/:ns/:name/affinity", workloadHandler.GetPodAffinity)
				workloads.PUT("/deployments/:ns/:name/affinity", workloadHandler.UpdatePodAffinity)

				// 环境克隆
				workloads.POST("/namespaces/clone", workloadHandler.CloneNamespace)
			}

			// 成本分析路由
			costHandler := workload.NewCostHandler(model.DB)
			costGroup := protected.Group("/clusters/:id/cost")
			{
				costGroup.GET("/config", costHandler.GetCostConfig)
				costGroup.POST("/config", costHandler.SaveCostConfig)
				costGroup.GET("/analysis", costHandler.GetResourceCost)
			}

			// 运维工具路由
			opsGroup := protected.Group("/ops/:id")
			{
				// P0: 资源使用趋势
				opsGroup.GET("/metrics/trend", opsHandler.GetResourceTrend)

				// P0: 事件时间线
				opsGroup.GET("/events/timeline", opsHandler.GetEventTimeline)

				// P0: 节点压力可视化
				opsGroup.GET("/nodes/pressure", opsHandler.GetNodePressure)

				// P0: 一键回滚
				opsGroup.POST("/rollback/deployment/:ns/:name", opsHandler.RollbackDeployment)

				// P1: 资源依赖图
				opsGroup.GET("/resource-graph", opsHandler.GetResourceGraph)

				// P1: RBAC 可视化
				opsGroup.GET("/rbac", opsHandler.GetRBACVisualization)

				// P1: 闲置资源清理
				opsGroup.GET("/idle-resources", opsHandler.FindIdleResources)
				opsGroup.POST("/idle-resources/clean", opsHandler.CleanIdleResource)
			}

			// Feature modules (aiops/inspection/eventforward/scheduler/backup/webhook/appstore)
			if modReg != nil {
				if err := modReg.RegisterRoutes(modCtx, protected); err != nil {
					logger.Warn("failed to register module routes", zap.Error(err))
				}
			}

			// 多租户管理
			tenantGroup := protected.Group("/tenants")
			{
				tenantGroup.GET("", tenantHandler.ListTenants)
				tenantGroup.POST("", tenantHandler.CreateTenant)
				tenantGroup.GET("/:id", tenantHandler.GetTenant)
				tenantGroup.PUT("/:id", tenantHandler.UpdateTenant)
				tenantGroup.DELETE("/:id", tenantHandler.DeleteTenant)
				tenantGroup.POST("/:id/members", tenantHandler.AddTenantMember)
				tenantGroup.DELETE("/:id/members/:userId", tenantHandler.RemoveTenantMember)
				tenantGroup.POST("/:id/namespaces", tenantHandler.CreateTenantNamespace)
				tenantGroup.DELETE("/:id/namespaces/:nsId", tenantHandler.DeleteTenantNamespace)
			}

		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Ticket issuance uses normal JWT authentication. The resulting ticket is
	// short-lived, single-use, and bound to the exact terminal target.
	r.POST(
		"/api/v1/ws/tickets/pod/:id/:ns/:name",
		middleware.AuthMiddleware(jwtManager),
		func(c *gin.Context) { c.Set("authz_grant_resolver", authorizer); c.Next() },
		middleware.PolicyAuthzMiddleware(authorizer),
		webSocketTicketHandler.IssuePod,
	)
	r.POST(
		"/api/v1/ws/tickets/node/:id/:name",
		middleware.AuthMiddleware(jwtManager),
		func(c *gin.Context) { c.Set("authz_grant_resolver", authorizer); c.Next() },
		middleware.PolicyAuthzMiddleware(authorizer),
		webSocketTicketHandler.IssueNode,
	)

	// WebSocket connections consume the ticket and repeat authorization checks
	// so role or cluster grants revoked between issuance and use take effect.
	r.GET(
		"/api/v1/ws/terminal/:id/:ns/:name",
		middleware.WebSocketTicketAuthMiddleware(webSocketTicketManager, "pod"),
		func(c *gin.Context) { c.Set("authz_grant_resolver", authorizer); c.Next() },
		middleware.PolicyAuthzMiddleware(authorizer),
		workloadHandler.PodTerminal,
	)
	r.GET(
		"/api/v1/ws/node-shell/:id/:name",
		middleware.WebSocketTicketAuthMiddleware(webSocketTicketManager, "node"),
		func(c *gin.Context) { c.Set("authz_grant_resolver", authorizer); c.Next() },
		middleware.PolicyAuthzMiddleware(authorizer),
		workloadHandler.NodeShell,
	)

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
