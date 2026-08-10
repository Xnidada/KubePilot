package wsticket

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubepilot/kubepilot/internal/pkg/cache"
)

const (
	ticketPrefix = "ws-ticket:"
	ticketTTL    = 30 * time.Second
)

type Claims struct {
	UserID       uint   `json:"user_id"`
	Kind         string `json:"kind"`
	ClusterID    uint   `json:"cluster_id"`
	Namespace    string `json:"namespace,omitempty"`
	ResourceName string `json:"resource_name"`
}

type Manager struct {
	cache cache.Cache
}

func NewManager(cacheInstance cache.Cache) *Manager {
	return &Manager{cache: cacheInstance}
}

func (m *Manager) Issue(ctx context.Context, claims Claims) (string, time.Duration, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", 0, fmt.Errorf("generate ticket: %w", err)
	}
	ticket := base64.RawURLEncoding.EncodeToString(randomBytes)
	if err := m.cache.Set(ctx, ticketPrefix+ticket, claims, ticketTTL); err != nil {
		return "", 0, fmt.Errorf("store ticket: %w", err)
	}
	return ticket, ticketTTL, nil
}

func (m *Manager) Consume(ctx context.Context, ticket string) (*Claims, error) {
	value, err := m.cache.Take(ctx, ticketPrefix+ticket)
	if err != nil {
		return nil, fmt.Errorf("consume ticket: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal([]byte(value), &claims); err != nil {
		return nil, fmt.Errorf("decode ticket: %w", err)
	}
	return &claims, nil
}
