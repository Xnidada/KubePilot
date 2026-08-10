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

const (
	maxAuditBodyBytes = 4096
	maxAuditTextBytes = 2048
)

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

var sensitiveFieldNames = map[string]struct{}{
	"password":      {},
	"new_password":  {},
	"old_password":  {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"secret":        {},
	"client_secret": {},
	"api_key":       {},
	"apikey":        {},
	"kubeconfig":    {},
	"private_key":   {},
	"credentials":   {},
	"authorization": {},
}

func normalizeFieldName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "-", "_"))
}

func isSensitiveField(name string) bool {
	_, ok := sensitiveFieldNames[normalizeFieldName(name)]
	return ok
}

func redactValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			if isSensitiveField(key) {
				out[key] = "******"
				continue
			}
			out[key] = redactValue(nested)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, nested := range typed {
			out[i] = redactValue(nested)
		}
		return out
	default:
		return value
	}
}

func maskSensitiveData(data []byte, path string) string {
	if len(data) == 0 {
		return ""
	}
	if strings.Contains(path, "/secrets") {
		return "[secret data masked]"
	}
	var payload interface{}
	if err := json.Unmarshal(data, &payload); err == nil {
		masked, err := json.Marshal(redactValue(payload))
		if err == nil {
			return truncateAuditText(string(masked), maxAuditBodyBytes)
		}
	}
	return truncateAuditText(string(data), maxAuditBodyBytes)
}

func truncateAuditText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		path := c.Request.URL.Path

		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		w := &responseBodyWriter{body: &bytes.Buffer{}, ResponseWriter: c.Writer}
		c.Writer = w

		c.Next()

		latency := time.Since(startTime).Milliseconds()
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		action := extractAction(c.Request.Method)
		resourceType := extractResourceType(path)
		if raw, ok := c.Get("authz_resource"); ok {
			if value, castOK := raw.(string); castOK && value != "" {
				resourceType = value
			}
		}
		if raw, ok := c.Get("authz_action"); ok {
			if value, castOK := raw.(string); castOK && value != "" {
				action = value
			}
		}

		var uid *uint
		if id, ok := userID.(uint); ok {
			uid = &id
		}
		uname, _ := username.(string)

		maskedBody := maskSensitiveData(requestBody, path)
		decision, _ := c.Get("authz_decision")
		reason, _ := c.Get("authz_reason")
		clusterRaw, _ := c.Get("authz_cluster_id")
		namespaceRaw, _ := c.Get("authz_namespace")

		var clusterID *uint
		if id, ok := clusterRaw.(uint); ok && id > 0 {
			clusterID = &id
		}
		namespace, _ := namespaceRaw.(string)

		resultParts := []string{}
		if decisionStr, ok := decision.(string); ok && decisionStr != "" {
			resultParts = append(resultParts, "decision="+decisionStr)
		}
		if reasonStr, ok := reason.(string); ok && reasonStr != "" {
			resultParts = append(resultParts, "reason="+reasonStr)
		}
		result := truncateAuditText(strings.Join(resultParts, "; "), maxAuditTextBytes)

		auditLog := &model.AuditLog{
			UserID:       uid,
			Username:     uname,
			Action:       action,
			ResourceType: resourceType,
			ResourceName: extractResourceName(c),
			ClusterID:    clusterID,
			Namespace:    namespace,
			RequestBody:  maskedBody,
			ResponseCode: c.Writer.Status(),
			Latency:      latency,
			IP:           c.ClientIP(),
			UserAgent:    truncateAuditText(c.Request.UserAgent(), 256),
			Success:      c.Writer.Status() < 400,
			Error:        truncateAuditText(c.Errors.ByType(gin.ErrorTypeAny).String(), maxAuditTextBytes),
			Result:       result,
			CreatedAt:    time.Now(),
		}

		go func(log *model.AuditLog) {
			if err := model.DB.Create(log).Error; err != nil {
				logger.Error("failed to save audit log", zap.Error(err))
			}
		}(auditLog)
	}
}

func extractAction(method string) string {
	switch method {
	case "GET":
		return "get"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func extractResourceType(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == "api" || part == "v1" {
			continue
		}
		if i+1 < len(parts) {
			return part
		}
		return part
	}
	return "unknown"
}

func extractResourceName(c *gin.Context) string {
	if name := c.Param("name"); name != "" {
		return name
	}
	if id := c.Param("id"); id != "" {
		return id
	}
	return ""
}
