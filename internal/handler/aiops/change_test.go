package aiops

import (
	"testing"

	"github.com/kubepilot/kubepilot/internal/model"
)

func TestProductionChangeSafetyStillAppliesWithoutDualApproval(t *testing.T) {
	h := &Handler{}
	evidence, err := h.productionChangeEvidence(model.AgentAction{Parameters: `{"action":"scale_deployment"}`})
	if err != nil || evidence != `{"source":"manual"}` {
		t.Fatalf("safe staged change rejected: evidence=%q err=%v", evidence, err)
	}
	for _, params := range []string{
		`{"action":"delete_deployment"}`,
		`{"action":"create_deployment","host_path_mounts":[{"host_path":"/tmp","mount_path":"/data"}]}`,
		`{"action":"update_deployment","env_vars":{"KEY":"value"}}`,
	} {
		if _, err := h.productionChangeEvidence(model.AgentAction{Parameters: params}); err == nil {
			t.Fatalf("unsafe production change accepted: %s", params)
		}
	}
}
