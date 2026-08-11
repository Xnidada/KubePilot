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
		"POST /api/v1/clusters/:id/workloads/batch",
		"POST /api/v1/clusters/:id/workloads/compare",
		"POST /api/v1/clusters/:id/workloads/yaml/apply",
		"POST /api/v1/aiops/agent",
		"POST /api/v1/aiops/agent/confirm/:actionId",
		"POST /api/v1/aiops/kubectl",
		"GET /api/v1/system/user-groups",
		"PUT /api/v1/system/user-groups/:id/members",
		"PUT /api/v1/system/user-groups/:id/clusters",
		"GET /api/v1/system/users/:id/effective-cluster-permissions",
		"GET /api/v1/system/users/:id/effective-access",
		"POST /api/v1/ws/tickets/pod/:id/:ns/:name",
		"GET /api/v1/inspection/rules/:id",
		"GET /api/v1/backups/:id",
		"DELETE /api/v1/backups/:id",
		"DELETE /api/v1/backups/restores/:id",
		"POST /api/v1/scheduler/tasks",
		"POST /api/v1/alerts/webhook/alertmanager",
		"GET /api/v1/alerts/rules",
		"POST /api/v1/alerts/channels",
	}
	for _, key := range required {
		parts := strings.SplitN(key, " ", 2)
		if !registry.Registered(parts[0], parts[1]) {
			t.Fatalf("missing policy for %s", key)
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
