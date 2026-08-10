package wsticket

import (
	"context"
	"testing"

	"github.com/kubepilot/kubepilot/internal/pkg/cache"
)

func TestTicketCanOnlyBeConsumedOnce(t *testing.T) {
	cacheInstance := cache.NewMemoryCache()
	t.Cleanup(func() {
		_ = cacheInstance.Close()
	})
	manager := NewManager(cacheInstance)
	expected := Claims{
		UserID:       42,
		Kind:         "pod",
		ClusterID:    7,
		Namespace:    "production",
		ResourceName: "api-0",
	}

	ticket, ttl, err := manager.Issue(context.Background(), expected)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if ticket == "" || ttl <= 0 {
		t.Fatal("ticket and positive TTL are required")
	}

	actual, err := manager.Consume(context.Background(), ticket)
	if err != nil {
		t.Fatalf("consume ticket: %v", err)
	}
	if *actual != expected {
		t.Fatalf("claims mismatch: got %+v, want %+v", *actual, expected)
	}

	if _, err := manager.Consume(context.Background(), ticket); err == nil {
		t.Fatal("second consumption should fail")
	}
}
