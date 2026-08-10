package module

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/config"
	"github.com/kubepilot/kubepilot/internal/pkg/cache"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MultiInstance describes how a module should behave under multiple replicas.
// Real leader election is not implemented yet; values are advisory metadata.
const (
	MultiInstanceAll        = "all"
	MultiInstanceLeaderOnly = "leader_only"
)

// Metadata describes a module for discovery and dependency ordering.
type Metadata struct {
	Name          string
	Version       string
	Description   string
	Dependencies  []string
	MultiInstance string
}

// MenuItem is a backend menu declaration for enabled modules.
type MenuItem struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Parent   string `json:"parent,omitempty"`
	Order    int    `json:"order,omitempty"`
	Resource string `json:"resource,omitempty"`
	Action   string `json:"action,omitempty"`
}

// PermissionDef documents resources a module contributes.
type PermissionDef struct {
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// Host exposes shared runtime dependencies to modules.
type Host struct {
	DB         *gorm.DB
	Config     *config.Config
	Cache      cache.Cache
	EncryptKey string
	Logger     *zap.Logger
}

// Context is passed when registering HTTP routes under the protected /api/v1 group.
type Context struct {
	Host *Host
}

// Module is an in-process feature unit (not a dynamically loaded plugin).
type Module interface {
	Meta() Metadata
	Migrations() []any
	RegisterPolicies(reg *authz.Registry)
	RegisterRoutes(ctx *Context, protected *gin.RouterGroup)
	Menus() []MenuItem
	Permissions() []PermissionDef
	Start(ctx context.Context, host *Host) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}

// Base provides no-op defaults for optional Module methods.
type Base struct{}

func (Base) Migrations() []any                            { return nil }
func (Base) RegisterPolicies(reg *authz.Registry)         {}
func (Base) Menus() []MenuItem                            { return nil }
func (Base) Permissions() []PermissionDef                 { return nil }
func (Base) Start(ctx context.Context, host *Host) error  { return nil }
func (Base) Stop(ctx context.Context) error               { return nil }
func (Base) Health(ctx context.Context) error             { return nil }

// Status is returned by the modules discovery API.
type Status struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Description   string         `json:"description"`
	Enabled       bool           `json:"enabled"`
	Healthy       bool           `json:"healthy"`
	HealthError   string         `json:"health_error,omitempty"`
	MultiInstance string         `json:"multi_instance,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
}

// StatusDetailer is an optional Module capability for runtime metrics
// exposed on GET /api/v1/modules.
type StatusDetailer interface {
	StatusDetails(ctx context.Context) map[string]any
}
