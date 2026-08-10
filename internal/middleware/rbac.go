package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
)

// RBACMiddleware RBAC权限检查中间件
func RBACMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取用户角色ID
		roleID, exists := c.Get("role_id")
		if !exists {
			response.Forbidden(c, "no role found")
			c.Abort()
			return
		}

		// 加载角色信息
		var role model.Role
		if err := model.DB.First(&role, roleID).Error; err != nil {
			response.Forbidden(c, "role not found")
			c.Abort()
			return
		}

		// 系统管理员角色拥有所有权限
		if role.IsSystem || role.Name == "admin" {
			c.Next()
			return
		}

		// 获取所需权限
		requiredResource, _ := c.Get("required_resource")
		requiredAction, _ := c.Get("required_action")

		if requiredResource == nil || requiredAction == nil {
			// 没有设置权限要求，允许访问
			c.Next()
			return
		}

		// 解析权限
		permissions, err := model.ParsePermissions(role.Permissions)
		if err != nil {
			response.Forbidden(c, "invalid permissions")
			c.Abort()
			return
		}

		// 检查权限
		if !permissions.HasPermission(requiredResource.(string), requiredAction.(string)) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequirePermission 设置路由所需权限
func RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("required_resource", resource)
		c.Set("required_action", action)
		c.Next()
	}
}

// RequireClusterAccess restricts a request to clusters explicitly assigned to
// the current user. System administrators retain global access.
func RequireClusterAccess(requiredLevel string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, roleExists := c.Get("role_id")
		userID, userExists := c.Get("user_id")
		if !roleExists || !userExists {
			response.Forbidden(c, "missing user authorization context")
			c.Abort()
			return
		}

		var role model.Role
		if err := model.DB.First(&role, roleID).Error; err != nil {
			response.Forbidden(c, "role not found")
			c.Abort()
			return
		}
		if role.IsSystem || role.Name == "admin" {
			c.Next()
			return
		}

		clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			response.BadRequest(c, "invalid cluster id")
			c.Abort()
			return
		}

		query := model.DB.Model(&model.UserCluster{}).
			Where("user_id = ? AND cluster_id = ?", userID, uint(clusterID))
		if namespace := c.Param("ns"); namespace != "" {
			query = query.Where("namespace IN ?", []string{"*", namespace})
		} else {
			query = query.Where("namespace = ?", "*")
		}

		switch requiredLevel {
		case "write":
			query = query.Where("permission_level IN ?", []string{"write", "admin"})
		case "admin":
			query = query.Where("permission_level = ?", "admin")
		case "read":
			query = query.Where("permission_level IN ?", []string{"read", "write", "admin"})
		default:
			response.Forbidden(c, "invalid cluster permission requirement")
			c.Abort()
			return
		}

		var count int64
		if err := query.Count(&count).Error; err != nil || count == 0 {
			response.Forbidden(c, "cluster access denied")
			c.Abort()
			return
		}

		c.Next()
	}
}

// AutoRBACMiddleware 自动RBAC权限检查中间件
func AutoRBACMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取用户角色ID
		roleID, exists := c.Get("role_id")
		if !exists {
			response.Forbidden(c, "no role found")
			c.Abort()
			return
		}

		// 加载角色信息
		var role model.Role
		if err := model.DB.First(&role, roleID).Error; err != nil {
			response.Forbidden(c, "role not found")
			c.Abort()
			return
		}

		// 系统管理员角色拥有所有权限
		if role.IsSystem || role.Name == "admin" {
			c.Next()
			return
		}

		// 自动提取资源和操作
		resource := extractResourceFromPath(c.FullPath())
		action := extractActionFromMethod(c.Request.Method)

		// 解析权限
		permissions, err := model.ParsePermissions(role.Permissions)
		if err != nil {
			response.Forbidden(c, "invalid permissions")
			c.Abort()
			return
		}

		// 检查权限
		if !permissions.HasPermission(resource, action) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// extractResourceFromPath 从路径中提取资源类型
func extractResourceFromPath(path string) string {
	resources := map[string]string{
		"/clusters":    "clusters",
		"/deployments": "deployments",
		"/pods":        "pods",
		"/services":    "services",
		"/configmaps":  "configmaps",
		"/secrets":     "secrets",
		"/pvcs":        "pvcs",
		"/pvs":         "pvs",
		"/namespaces":  "namespaces",
		"/nodes":       "nodes",
		"/events":      "events",
		"/alerts":      "alerts",
		"/users":       "users",
		"/roles":       "roles",
		"/audit-logs":  "audit_logs",
		"/appstore":    "appstore",
	}

	for pattern, resource := range resources {
		if strings.Contains(path, pattern) {
			return resource
		}
	}
	return "unknown"
}

// extractActionFromMethod 从HTTP方法中提取操作类型
func extractActionFromMethod(method string) string {
	switch method {
	case "GET":
		return "view"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "edit"
	case "DELETE":
		return "delete"
	default:
		return "view"
	}
}
