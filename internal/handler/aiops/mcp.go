package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	aiopsService "github.com/kubepilot/kubepilot/internal/service/aiops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

type mcpPrincipalKey struct{}

type mcpPrincipal struct {
	userID     uint
	roleID     uint
	username   string
	ip         string
	userAgent  string
	authorizer *authz.Authorizer
}

type mcpScopeInput struct {
	ClusterID uint   `json:"cluster_id" jsonschema:"KubePilot cluster ID"`
	Namespace string `json:"namespace" jsonschema:"Explicit Kubernetes namespace; wildcards are not allowed"`
}

type mcpWorkloadInput struct {
	mcpScopeInput
	ResourceType  string `json:"resource_type" jsonschema:"One of pods, deployments, services"`
	NamePrefix    string `json:"name_prefix,omitempty" jsonschema:"Optional resource name prefix"`
	LabelSelector string `json:"label_selector,omitempty" jsonschema:"Optional Kubernetes label selector"`
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum displayed items, up to 50"`
}

type mcpEventsInput struct {
	mcpScopeInput
	Name  string `json:"name,omitempty" jsonschema:"Optional involved resource name"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum displayed items, up to 50"`
}

// NewReadOnlyMCPHandler serves stateless MCP over HTTP using the existing user JWT.
// Only these two explicit query tools are registered; no mutation tool is reachable.
func NewReadOnlyMCPHandler(db *gorm.DB) gin.HandlerFunc {
	queries := aiopsService.NewReadOnlyService(db)
	server := mcp.NewServer(&mcp.Implementation{Name: "kubepilot-readonly", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_workloads", Description: "List Pod, Deployment or Service status in one authorized namespace.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpWorkloadInput) (*mcp.CallToolResult, any, error) {
		resource := strings.ToLower(strings.TrimSpace(input.ResourceType))
		if resource != "pods" && resource != "deployments" && resource != "services" {
			return nil, nil, fmt.Errorf("resource_type must be pods, deployments, or services")
		}
		args, _ := json.Marshal(input)
		return mcpReadResult(runMCPRead(ctx, db, queries, "list_workloads", resource, input.ClusterID, input.Namespace, string(args)))
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_events", Description: "List Kubernetes events in one authorized namespace, optionally for one resource.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpEventsInput) (*mcp.CallToolResult, any, error) {
		args, _ := json.Marshal(input)
		return mcpReadResult(runMCPRead(ctx, db, queries, "list_events", "events", input.ClusterID, input.Namespace, string(args)))
	})
	transport := http.NewCrossOriginProtection().Handler(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 64 << 10},
	))
	return func(c *gin.Context) {
		userID, userOK := c.Get("user_id")
		roleID, roleOK := c.Get("role_id")
		authorizer, authzOK := authz.FromContext(c)
		if !userOK || !roleOK || !authzOK {
			response.Forbidden(c, "MCP user authorization context is unavailable")
			return
		}
		username, _ := c.Get("username")
		name, _ := username.(string)
		principal := mcpPrincipal{
			userID: userID.(uint), roleID: roleID.(uint), username: name,
			ip: c.ClientIP(), userAgent: c.Request.UserAgent(), authorizer: authorizer,
		}
		request := c.Request.WithContext(context.WithValue(c.Request.Context(), mcpPrincipalKey{}, principal))
		transport.ServeHTTP(c.Writer, request)
	}
}

func runMCPRead(ctx context.Context, db *gorm.DB, queries *aiopsService.Service, tool, resource string, clusterID uint, namespace, args string) (string, error) {
	principal, ok := ctx.Value(mcpPrincipalKey{}).(mcpPrincipal)
	if !ok || principal.authorizer == nil {
		return "", fmt.Errorf("MCP user context is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	start := time.Now()
	authErr := principal.authorizer.Check(ctx, principal.userID, principal.roleID, resource, "view", clusterID, namespace)
	err := authErr
	var result string
	if err == nil {
		result, err = queries.ExecuteMCPReadOnly(ctx, principal.userID, clusterID, tool, args)
	}
	if db != nil {
		status := http.StatusOK
		if authErr != nil {
			status = http.StatusForbidden
		} else if err != nil {
			status = http.StatusInternalServerError
		}
		entry := model.AuditLog{
			UserID: &principal.userID, Username: principal.username, Action: "MCP_CALL",
			ResourceType: "mcp", ResourceName: tool, ClusterID: &clusterID, Namespace: namespace,
			ResponseCode: status, Latency: time.Since(start).Milliseconds(), IP: principal.ip,
			UserAgent: principal.userAgent, Success: err == nil,
		}
		auditCtx, stopAudit := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stopAudit()
		if auditErr := db.WithContext(auditCtx).Create(&entry).Error; auditErr != nil {
			return "", fmt.Errorf("MCP audit unavailable")
		}
	}
	return result, err
}

func mcpReadResult(text string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}
