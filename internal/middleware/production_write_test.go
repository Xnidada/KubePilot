package middleware

import "testing"

func TestDirectClusterMutation(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/api/v1/clusters/:id/workloads/yaml/apply", true},
		{"DELETE", "/api/v1/clusters/:id/workloads/secrets/:ns/:name", true},
		{"POST", "/api/v1/ops/:id/rollback/deployment/:ns/:name", true},
		{"GET", "/api/v1/ws/terminal/:id/:ns/:name", true},
		{"POST", "/api/v1/ws/tickets/node/:id/:name", true},
		{"GET", "/api/v1/clusters/:id/workloads/pods", false},
		{"PUT", "/api/v1/clusters/:id", false},
		{"POST", "/api/v1/aiops/agent/confirm/:actionId", false},
	} {
		if got := directClusterMutation(tc.method, tc.path); got != tc.want {
			t.Fatalf("%s %s: got %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
