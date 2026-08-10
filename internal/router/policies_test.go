package router

import (
	"strings"
	"testing"

	"github.com/kubepilot/kubepilot/internal/authz"
)

func TestProtectedRoutesHaveExplicitPolicies(t *testing.T) {
	registry := authz.NewRegistry()
	registerAPIPolicies(registry)

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
		"POST /api/v1/ws/tickets/pod/:id/:ns/:name",
		"GET /api/v1/inspection/rules/:id",
		"GET /api/v1/backups/:id",
		"POST /api/v1/scheduler/tasks",
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
