package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
)

// PolicyAuthzMiddleware applies the fail-closed route policy authorizer.
func PolicyAuthzMiddleware(authorizer *authz.Authorizer) gin.HandlerFunc {
	if authorizer == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(500, gin.H{"code": 500, "message": "authorizer not configured"})
		}
	}
	return authorizer.Middleware()
}
