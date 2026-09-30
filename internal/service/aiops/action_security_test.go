package aiops

import (
	"strings"
	"testing"
)

func TestActionParametersAreSealedAndTraceIsRedacted(t *testing.T) {
	s := &Service{encryptKey: "test-encryption-key"}
	raw := `{"action":"create_secret","data":{"password":"test-sensitive-value"}}`
	sealed, err := s.sealActionParameters(raw)
	if err != nil || strings.Contains(sealed, "test-sensitive-value") {
		t.Fatalf("action parameters were not sealed: %v", err)
	}
	opened, err := s.openActionParameters(sealed)
	if err != nil || string(opened) != raw {
		t.Fatalf("sealed action did not round-trip: %v", err)
	}
	if strings.Contains(safeAgentTraceArgs("stage_mutation", raw), "test-sensitive-value") {
		t.Fatal("mutating tool trace exposed a secret")
	}
	if strings.Contains(safeAgentTraceResult("stage_mutation", "invalid value: test-sensitive-value", true), "test-sensitive-value") {
		t.Fatal("failed mutating tool trace exposed a secret")
	}
}
