package workload

import "testing"

func TestTerminalSessionRegistryLimitsPerUser(t *testing.T) {
	registry := terminalSessionRegistry{byUser: make(map[uint]int)}
	userID := uint(42)

	for i := 0; i < maxTerminalSessionsPerUser; i++ {
		if !registry.acquire(userID) {
			t.Fatalf("session %d should be allowed", i+1)
		}
	}
	if registry.acquire(userID) {
		t.Fatal("session above per-user limit should be rejected")
	}

	registry.release(userID)
	if !registry.acquire(userID) {
		t.Fatal("released capacity should be reusable")
	}
}
