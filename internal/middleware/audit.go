package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/logger"
	"github.com/kubepilot/kubepilot/internal/pkg/netutil"
	"go.uber.org/zap"
)

func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
			c.Next()
			return
		}
		startTime := time.Now()
		// Audit only metadata: free-form Agent prompts, YAML and tool results may
		// contain secrets, and reading bodies here can exhaust memory.
		batchIDs := safeBatchIDs(c.Request)
		c.Next()

		latency := time.Since(startTime).Milliseconds()

		// Get user info
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		// Extract resource info from path
		resourceType := extractResourceType(c.FullPath())
		resourceName := c.Param("name")
		if resourceName == "" && c.Request.Method == "DELETE" {
			resourceName = c.Param("id")
		}
		if resourceName == "" {
			resourceName = batchIDs
		}
		clusterID := ""
		if strings.Contains(c.FullPath(), "/clusters/:id") {
			clusterID = c.Param("id")
		}
		namespace := c.Param("ns")

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
			RequestBody:  "",
			ResponseCode: c.Writer.Status(),
			Latency:      latency,
			IP:           netutil.RealClientIP(c),
			UserAgent:    c.Request.UserAgent(),
			Success:      c.Writer.Status() < 400,
		}

		// 设置用户信息
		if userID != nil {
			id := userID.(uint)
			auditLog.UserID = &id
			auditLog.Username = username.(string)
		} else {
			auditLog.Username = "anonymous"
		}

		// 设置集群ID（如果存在）
		if clusterIDUint > 0 {
			auditLog.ClusterID = &clusterIDUint
		}

		persist := func() {
			if err := model.DB.Create(&auditLog).Error; err != nil {
				logger.Error("failed to save audit log", zap.Error(err))
			}
		}
		// Persist mutations before returning to the client. Reads remain async.
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			go persist()
		} else {
			persist()
		}
	}
}

// safeBatchIDs captures only the integer IDs of known batch-delete requests.
// Reading is bounded, and the original body stream is restored for handlers.
func safeBatchIDs(request *http.Request) string {
	path := request.URL.Path
	if request.Body == nil || request.Method != http.MethodPost ||
		(!strings.HasSuffix(path, "/batch-delete") && !strings.HasSuffix(path, "/batch-forget")) {
		return ""
	}
	original := request.Body
	const maxBody = 4096
	prefix, err := io.ReadAll(io.LimitReader(original, maxBody+1))
	request.Body = struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(bytes.NewReader(prefix), original), Closer: original}
	if err != nil || len(prefix) > maxBody {
		return ""
	}
	var body struct {
		IDs []uint `json:"ids"`
	}
	if json.Unmarshal(prefix, &body) != nil || len(body.IDs) == 0 || len(body.IDs) > 100 {
		return ""
	}
	ids := body.IDs
	if len(ids) > 8 {
		ids = ids[:8]
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprint(id))
	}
	name := "ids=" + strings.Join(parts, ",")
	if len(body.IDs) > len(ids) {
		name += fmt.Sprintf("(+%d)", len(body.IDs)-len(ids))
	}
	if len(name) > 128 {
		return name[:128]
	}
	return name
}

func extractResourceType(path string) string {
	if strings.HasPrefix(path, "/api/v1/backups/restores") {
		return "restore_records"
	}
	if path == "/api/v1/backups/:id" || path == "/api/v1/backups/batch-delete" {
		return "backup_records"
	}
	if strings.HasPrefix(path, "/api/v1/aiops/memories") {
		return "agent_memories"
	}
	if strings.HasPrefix(path, "/api/v1/webhooks/logs") {
		return "webhook_logs"
	}
	if strings.HasPrefix(path, "/api/v1/event-forward/logs") {
		return "event_forward_logs"
	}
	if strings.HasPrefix(path, "/api/v1/alerts/history") {
		return "alert_history"
	}
	if strings.HasPrefix(path, "/api/v1/inspection/reports") {
		return "inspection_reports"
	}
	if strings.Contains(path, "/login-logs") {
		return "login_logs"
	}
	if strings.Contains(path, "/oauth/configs") {
		return "oauth_configs"
	}
	resources := []string{"clusters", "deployments", "pods", "services", "configmaps", "secrets", "namespaces", "nodes", "ingresses", "jobs", "cronjobs", "statefulsets", "daemonsets", "users", "roles", "audit-logs"}
	for _, r := range resources {
		if strings.Contains(path, r) {
			return r
		}
	}
	return "unknown"
}
