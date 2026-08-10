package middleware

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"github.com/kubepilot/kubepilot/internal/pkg/utils"
	"github.com/kubepilot/kubepilot/internal/pkg/wsticket"
)

func AuthMiddleware(jwtManager *utils.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "invalid authorization header format")
			c.Abort()
			return
		}

		if !authenticateToken(c, jwtManager, parts[1]) {
			return
		}
		c.Next()
	}
}

// WebSocketTicketAuthMiddleware consumes a short-lived, single-use ticket and
// verifies that it was issued for the exact WebSocket target.
func WebSocketTicketAuthMiddleware(manager *wsticket.Manager, kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ticket := c.Query("ticket")
		if ticket == "" {
			response.Unauthorized(c, "missing websocket ticket")
			c.Abort()
			return
		}

		claims, err := manager.Consume(c.Request.Context(), ticket)
		if err != nil {
			response.Unauthorized(c, "invalid or expired websocket ticket")
			c.Abort()
			return
		}
		clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil ||
			claims.Kind != kind ||
			claims.ClusterID != uint(clusterID) ||
			claims.Namespace != c.Param("ns") ||
			claims.ResourceName != c.Param("name") {
			response.Unauthorized(c, "websocket ticket target mismatch")
			c.Abort()
			return
		}
		if !authenticateUser(c, claims.UserID) {
			return
		}
		c.Next()
	}
}

func authenticateToken(c *gin.Context, jwtManager *utils.JWTManager, token string) bool {
	claims, err := jwtManager.ParseToken(token)
	if err != nil {
		response.Unauthorized(c, "invalid or expired token")
		c.Abort()
		return false
	}

	return authenticateUser(c, claims.UserID)
}

func authenticateUser(c *gin.Context, userID uint) bool {
	var user model.User
	if err := model.DB.Select("id", "username", "role_id", "status").First(&user, userID).Error; err != nil || user.Status != 1 {
		response.Unauthorized(c, "user is disabled or no longer exists")
		c.Abort()
		return false
	}

	c.Set("user_id", user.ID)
	c.Set("username", user.Username)
	c.Set("role_id", user.RoleID)
	return true
}
