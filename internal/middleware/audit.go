package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"go.uber.org/zap"
)

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Read request body
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Wrap response writer
		w := &responseBodyWriter{
			body:           &bytes.Buffer{},
			ResponseWriter: c.Writer,
		}
		c.Writer = w

		c.Next()

		latency := time.Since(startTime).Milliseconds()

		// Get user info
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		// Extract resource info from path
		resourceType := extractResourceType(c.FullPath())
		resourceName := c.Param("name")
		clusterID := c.Param("id")
		namespace := c.Param("ns")

		// Parse cluster ID
		var clusterIDUint uint
		if clusterID != "" {
			// Convert string to uint (simplified)
			for _, c := range clusterID {
				if c >= '0' && c <= '9' {
					clusterIDUint = clusterIDUint*10 + uint(c-'0')
				}
			}
		}

		auditLog := model.AuditLog{
			Action:       c.Request.Method,
			ResourceType: resourceType,
			ResourceName: resourceName,
			ClusterID:    clusterIDUint,
			Namespace:    namespace,
			RequestBody:  string(requestBody),
			ResponseCode: c.Writer.Status(),
			Latency:      latency,
			IP:           c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			Success:      c.Writer.Status() < 400,
		}

		if userID != nil {
			auditLog.UserID = userID.(uint)
			auditLog.Username = username.(string)
		}

		// Save audit log asynchronously
		go func() {
			if err := model.DB.Create(&auditLog).Error; err != nil {
				logger.Error("failed to save audit log", zap.Error(err))
			}
		}()
	}
}

func extractResourceType(path string) string {
	// Extract resource type from API path
	// e.g., /api/v1/clusters/:id/deployments -> deployments
	resources := []string{"clusters", "deployments", "pods", "services", "configmaps", "secrets", "namespaces", "nodes", "ingresses", "jobs", "cronjobs", "statefulsets", "daemonsets"}
	for _, r := range resources {
		if contains(path, r) {
			return r
		}
	}
	return "unknown"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
