package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
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

// sensitivePaths 需要脱敏的路径
var sensitivePaths = []string{
	"/auth/login",
	"/auth/register",
	"/secrets",
	"/aiops/configs",
	"/profile/password",
}

// isSensitivePath 检查是否是敏感路径
func isSensitivePath(path string) bool {
	for _, sp := range sensitivePaths {
		if strings.Contains(path, sp) {
			return true
		}
	}
	return false
}

// maskSensitiveData 脱敏请求体
func maskSensitiveData(data []byte, path string) string {
	if len(data) == 0 {
		return ""
	}

	// 对于登录/注册请求，隐藏密码
	if strings.Contains(path, "/auth/") {
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err == nil {
			if _, ok := m["password"]; ok {
				m["password"] = "******"
			}
			if masked, err := json.Marshal(m); err == nil {
				return string(masked)
			}
		}
		return "[masked]"
	}

	// 对于 Secret 操作，不记录内容
	if strings.Contains(path, "/secrets") {
		return "[secret data masked]"
	}

	// 对于 LLM 配置，隐藏 API Key
	if strings.Contains(path, "/aiops/configs") {
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err == nil {
			if _, ok := m["api_key"]; ok {
				m["api_key"] = "******"
			}
			if masked, err := json.Marshal(m); err == nil {
				return string(masked)
			}
		}
		return "[masked]"
	}

	// 限制请求体大小
	if len(data) > 4096 {
		return string(data[:4096]) + "...[truncated]"
	}

	return string(data)
}

func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		path := c.Request.URL.Path

		// 读取请求体
		var requestBody []byte
		if c.Request.Body != nil && !isSensitivePath(path) {
			// 非敏感路径，读取并记录
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		} else if c.Request.Body != nil {
			// 敏感路径，读取但脱敏
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

		// Mask request body for sensitive paths
		maskedBody := maskSensitiveData(requestBody, path)

		// Parse cluster ID
		var clusterIDUint uint
		if clusterID != "" {
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
			Namespace:    namespace,
			RequestBody:  maskedBody,
			ResponseCode: c.Writer.Status(),
			Latency:      latency,
			IP:           c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			Success:      c.Writer.Status() < 400,
		}

		// 设置用户信息
		if userID != nil {
			auditLog.UserID = userID.(uint)
			auditLog.Username = username.(string)
		} else {
			auditLog.UserID = 1
			auditLog.Username = "anonymous"
		}

		// 设置集群ID（如果存在）
		if clusterIDUint > 0 {
			auditLog.ClusterID = clusterIDUint
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
	resources := []string{"clusters", "deployments", "pods", "services", "configmaps", "secrets", "namespaces", "nodes", "ingresses", "jobs", "cronjobs", "statefulsets", "daemonsets", "users", "roles", "audit-logs"}
	for _, r := range resources {
		if strings.Contains(path, r) {
			return r
		}
	}
	return "unknown"
}
