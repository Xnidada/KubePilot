package aiops

import (
	"context"
	"strings"
	"testing"
)

func TestMCPReadOnlyRejectsWritesSecretsAndUnscopedQueries(t *testing.T) {
	svc := NewReadOnlyService(nil)
	for _, tc := range []struct {
		name, args string
		clusterID  uint
		want       string
	}{
		{"stage_mutation", `{"namespace":"default"}`, 1, "not allowed"},
		{"list_workloads", `{"namespace":"default","resource_type":"secrets"}`, 1, "resource_type"},
		{"list_workloads", `{"namespace":"*","resource_type":"pods"}`, 1, "concrete namespace"},
		{"list_events", `{"namespace":""}`, 1, "concrete namespace"},
		{"list_events", `{"namespace":"default"}`, 0, "cluster_id"},
	} {
		_, err := svc.ExecuteMCPReadOnly(context.Background(), 1, tc.clusterID, tc.name, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %s: got %v, want %q", tc.name, tc.args, err, tc.want)
		}
	}
}
