package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
)

// PolicyAuthzMiddleware enforces explicit route policies (fail closed).
func PolicyAuthzMiddleware(authorizer *authz.Authorizer) gin.HandlerFunc {
	return authorizer.Middleware()
}

// RequirePermission sets route-required platform permissions for ad-hoc middleware chains.
func RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("required_resource", resource)
		c.Set("required_action", action)
		c.Next()
	}
}

// RBACMiddleware checks required_resource/required_action against the caller role.
func RBACMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, exists := c.Get("role_id")
		if !exists {
			response.Forbidden(c, "no role found")
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

		requiredResource, _ := c.Get("required_resource")
		requiredAction, _ := c.Get("required_action")
		if requiredResource == nil || requiredAction == nil {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		permissions, err := model.ParsePermissions(role.Permissions)
		if err != nil {
			response.Forbidden(c, "invalid permissions")
			c.Abort()
			return
		}
		if !permissions.HasPermission(requiredResource.(string), requiredAction.(string)) {
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireClusterAccess restricts requests using the effective grant resolver when available.
// Prefer PolicyAuthzMiddleware for new routes.
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

		namespace := c.Param("ns")
		if namespace == "" {
			namespace = "*"
		}

		resolver, ok := c.Get("authz_grant_resolver")
		if ok {
			if grantResolver, castOK := resolver.(authz.GrantResolver); castOK {
				allowed, err := grantResolver.Authorize(c.Request.Context(), userID.(uint), uint(clusterID), namespace, requiredLevel)
				if err != nil || !allowed {
					response.Forbidden(c, "cluster access denied")
					c.Abort()
					return
				}
				c.Next()
				return
			}
		}

		query := model.DB.Model(&model.UserCluster{}).
			Where("user_id = ? AND cluster_id = ?", userID, uint(clusterID))
		if namespace != "*" {
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
