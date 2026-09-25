package aiops

import "testing"

func TestUsageCostEstimate(t *testing.T) {
	if got := usageCostEstimate(1_000_000, 500_000, 3, 6); got != 6 {
		t.Fatalf("cost = %v, want 6", got)
	}
	if got := usageCostEstimate(100, 100, 0, 0); got != 0 {
		t.Fatalf("free model cost = %v, want 0", got)
	}
}
