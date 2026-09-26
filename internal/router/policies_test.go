package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/module"
	"github.com/kubepilot/kubepilot/internal/modules"
)

func TestProtectedRoutesHaveExplicitPolicies(t *testing.T) {
	registry := authz.NewRegistry()
	registerAPIPolicies(registry)
	modReg := module.NewRegistry(nil, nil)
	modules.RegisterAll(modReg)
	if err := modReg.RegisterPolicies(registry); err != nil {
		t.Fatal(err)
	}

	required := []string{
		"GET /api/v1/clusters",
		"GET /api/v1/clusters/:id/workloads/deployments",
		"GET /api/v1/clusters/:id/workloads/deployments/:ns/:name",
		"GET /api/v1/clusters/:id/workloads/gateway-api",
		"GET /api/v1/clusters/:id/workloads/gateway-api/install-plan",
		"POST /api/v1/clusters/:id/workloads/gateway-api/install",
		"POST /api/v1/clusters/:id/workloads/batch",
		"POST /api/v1/clusters/:id/workloads/yaml/apply",
		"POST /api/v1/aiops/agent",
		"GET /api/v1/aiops/mcp",
		"POST /api/v1/aiops/mcp",
		"DELETE /api/v1/aiops/mcp",
		"POST /api/v1/aiops/agent/confirm/:actionId",
		"GET /api/v1/aiops/approval-settings",
		"PUT /api/v1/aiops/approval-settings",
		"POST /api/v1/aiops/kubectl",
		"GET /api/v1/system/user-groups",
		"PUT /api/v1/system/user-groups/:id/members",
		"PUT /api/v1/system/user-groups/:id/clusters",
		"GET /api/v1/system/users/:id/effective-cluster-permissions",
		"GET /api/v1/system/users/:id/effective-access",
		"DELETE /api/v1/system/login-logs/:id",
		"POST /api/v1/system/login-logs/batch-delete",
		"GET /api/v1/system/oauth/configs",
		"POST /api/v1/system/oauth/configs",
		"PUT /api/v1/system/oauth/configs/:id",
		"DELETE /api/v1/system/oauth/configs/:id",
		"POST /api/v1/ws/tickets/pod/:id/:ns/:name",
		"GET /api/v1/inspection/rules/:id",
		"GET /api/v1/backups/:id",
		"DELETE /api/v1/backups/:id",
		"POST /api/v1/backups/batch-delete",
		"DELETE /api/v1/backups/restores/:id",
		"POST /api/v1/backups/restores/batch-delete",
		"POST /api/v1/aiops/memories/batch-forget",
		"DELETE /api/v1/webhooks/logs/:id",
		"POST /api/v1/webhooks/logs/batch-delete",
		"DELETE /api/v1/event-forward/logs/:id",
		"POST /api/v1/event-forward/logs/batch-delete",
		"DELETE /api/v1/alerts/history/:id",
		"POST /api/v1/alerts/history/batch-delete",
		"DELETE /api/v1/inspection/reports/:id",
		"POST /api/v1/inspection/reports/batch-delete",
		"POST /api/v1/scheduler/tasks",
	}
	for _, key := range required {
		parts := strings.SplitN(key, " ", 2)
		if !registry.Registered(parts[0], parts[1]) {
			t.Fatalf("missing policy for %s", key)
		}
	}
	if policy, _ := registry.Lookup("GET", "/api/v1/clusters/:id/workloads/gateway-api"); policy.Resource != "custom_resources" || policy.Action != "view" || policy.Scope != authz.ScopeNamespaceList || !policy.AllowFilteredNamespaceList {
		t.Fatalf("unsafe Gateway API policy: %#v", policy)
	}
	for _, key := range []string{"GET /api/v1/clusters/:id/workloads/gateway-api/install-plan", "POST /api/v1/clusters/:id/workloads/gateway-api/install"} {
		parts := strings.SplitN(key, " ", 2)
		policy, _ := registry.Lookup(parts[0], parts[1])
		if policy.Resource != "clusters" || policy.Action != "admin" || policy.Scope != authz.ScopeCluster {
			t.Fatalf("unsafe Gateway API install policy for %s: %#v", key, policy)
		}
	}
	for _, key := range []string{
		"GET /api/v1/aiops/mcp",
		"POST /api/v1/aiops/mcp",
		"DELETE /api/v1/aiops/mcp",
	} {
		parts := strings.SplitN(key, " ", 2)
		policy, _ := registry.Lookup(parts[0], parts[1])
		if policy.Resource != "aiops" || policy.Action != "view" || policy.Scope != authz.ScopePlatform {
			t.Fatalf("unsafe MCP policy for %s: %#v", key, policy)
		}
	}
	if policy, _ := registry.Lookup("PUT", "/api/v1/aiops/approval-settings"); policy.Resource != "aiops_config" || policy.Action != "admin" || policy.Scope != authz.ScopePlatform {
		t.Fatalf("unsafe approval setting policy: %#v", policy)
	}
	if policy, _ := registry.Lookup("GET", "/api/v1/aiops/approval-settings"); !policy.AuthenticatedOnly {
		t.Fatalf("approval setting must require authentication: %#v", policy)
	}
	for _, key := range []string{
		"DELETE /api/v1/inspection/reports/:id",
		"POST /api/v1/inspection/reports/batch-delete",
	} {
		parts := strings.SplitN(key, " ", 2)
		policy, _ := registry.Lookup(parts[0], parts[1])
		if policy.Resource != "inspection" || policy.Action != "delete" || policy.Scope != authz.ScopeHandler {
			t.Fatalf("unsafe inspection report deletion policy for %s: %#v", key, policy)
		}
	}
	for _, key := range []string{
		"DELETE /api/v1/backups/:id",
		"POST /api/v1/backups/batch-delete",
		"DELETE /api/v1/backups/restores/:id",
		"POST /api/v1/backups/restores/batch-delete",
	} {
		parts := strings.SplitN(key, " ", 2)
		policy, _ := registry.Lookup(parts[0], parts[1])
		if policy.Resource != "backups" || policy.Action != "delete" || policy.Scope != authz.ScopeHandler {
			t.Fatalf("unsafe backup deletion policy for %s: %#v", key, policy)
		}
	}
	for _, key := range []string{
		"DELETE /api/v1/system/login-logs/:id",
		"POST /api/v1/system/login-logs/batch-delete",
	} {
		parts := strings.SplitN(key, " ", 2)
		policy, _ := registry.Lookup(parts[0], parts[1])
		if policy.Resource != "login_logs" || policy.Action != "delete" || policy.Scope != authz.ScopePlatform {
			t.Fatalf("unsafe login log deletion policy for %s: %#v", key, policy)
		}
	}
	for _, key := range []string{
		"GET /api/v1/system/oauth/configs",
		"POST /api/v1/system/oauth/configs",
		"PUT /api/v1/system/oauth/configs/:id",
		"DELETE /api/v1/system/oauth/configs/:id",
	} {
		parts := strings.SplitN(key, " ", 2)
		policy, _ := registry.Lookup(parts[0], parts[1])
		if policy.Resource != "users" || policy.Action != "admin" || policy.Scope != authz.ScopePlatform {
			t.Fatalf("unsafe OAuth config policy for %s: %#v", key, policy)
		}
	}

	keys := registry.Keys()
	if len(keys) < 200 {
		t.Fatalf("expected a large explicit policy set, got %d", len(keys))
	}
	for _, key := range keys {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid policy key %q", key)
		}
		policy, ok := registry.Lookup(parts[0], parts[1])
		if !ok {
			t.Fatalf("lookup failed for %s", key)
		}
		if policy.AuthenticatedOnly {
			continue
		}
		if policy.Resource == "" || policy.Action == "" {
			t.Fatalf("policy %s missing resource/action", key)
		}
		if policy.Scope == "" {
			t.Fatalf("policy %s missing scope", key)
		}
	}
}

func TestPolicyRegistryCoversProtectedAPIInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := authz.NewRegistry()
	registerAPIPolicies(registry)
	modReg := module.NewRegistry(nil, nil)
	modules.RegisterAll(modReg)
	if err := modReg.RegisterPolicies(registry); err != nil {
		t.Fatal(err)
	}

	// Build a minimal route inventory from registered policies and ensure each can be looked up
	// via Gin's FullPath semantics (same path templates).
	engine := gin.New()
	api := engine.Group("/api/v1")
	for _, key := range registry.Keys() {
		parts := strings.SplitN(key, " ", 2)
		method, fullPath := parts[0], parts[1]
		if !strings.HasPrefix(fullPath, "/api/v1/") {
			t.Fatalf("unexpected policy path %s", fullPath)
		}
		rel := strings.TrimPrefix(fullPath, "/api/v1")
		handler := func(c *gin.Context) { c.Status(http.StatusNoContent) }
		switch method {
		case http.MethodGet:
			api.GET(rel, handler)
		case http.MethodPost:
			api.POST(rel, handler)
		case http.MethodPut:
			api.PUT(rel, handler)
		case http.MethodPatch:
			api.PATCH(rel, handler)
		case http.MethodDelete:
			api.DELETE(rel, handler)
		default:
			t.Fatalf("unsupported method in policy %s", key)
		}
	}

	routes := engine.Routes()
	if len(routes) != len(registry.Keys()) {
		t.Fatalf("gin routes %d != policy keys %d", len(routes), len(registry.Keys()))
	}
	for _, route := range routes {
		if !registry.Registered(route.Method, route.Path) {
			t.Fatalf("registered gin route missing policy: %s %s", route.Method, route.Path)
		}
		req := httptest.NewRequest(route.Method, route.Path, nil)
		_ = req
	}
}
