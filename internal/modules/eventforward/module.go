package eventforward

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/config"
	efHandler "github.com/kubepilot/kubepilot/internal/handler/eventforward"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/module"
	"go.uber.org/zap"
)

type Module struct {
	module.Base
	handler *efHandler.EventForwardHandler
	cfg     config.ModuleConfig

	mu        sync.Mutex
	failSince time.Time // first time fail-rate condition was observed
}

func New() *Module { return &Module{} }

func (m *Module) Meta() module.Metadata {
	return module.Metadata{
		Name:          "eventforward",
		Version:       "1.0.0",
		Description:   "Kubernetes event forwarding rules",
		MultiInstance: module.MultiInstanceLeaderOnly,
	}
}

func (m *Module) Migrations() []any {
	return []any{
		&model.EventForwardRule{},
		&model.EventForwardLog{},
	}
}

func (m *Module) Permissions() []module.PermissionDef {
	return []module.PermissionDef{
		{Resource: "event_forward", Actions: []string{"view", "create", "edit", "delete", "execute"}},
	}
}

func (m *Module) Menus() []module.MenuItem {
	return []module.MenuItem{
		{Key: "/cluster/event-forward", Label: "Event 转发", Parent: "/ops", Order: 20, Resource: "event_forward", Action: "view"},
	}
}

func (m *Module) RegisterPolicies(reg *authz.Registry) {
	reg.MustRegister("GET", "/api/v1/event-forward/rules", authz.Policy{Resource: "event_forward", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/event-forward/rules", authz.Policy{Resource: "event_forward", Action: "create", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/event-forward/rules/:id", authz.Policy{Resource: "event_forward", Action: "view", Scope: authz.ScopeHandler})
	reg.MustRegister("PUT", "/api/v1/event-forward/rules/:id", authz.Policy{Resource: "event_forward", Action: "edit", Scope: authz.ScopeHandler})
	reg.MustRegister("DELETE", "/api/v1/event-forward/rules/:id", authz.Policy{Resource: "event_forward", Action: "delete", Scope: authz.ScopeHandler})
	reg.MustRegister("POST", "/api/v1/event-forward/rules/:id/test", authz.Policy{Resource: "event_forward", Action: "execute", Scope: authz.ScopeHandler})
	reg.MustRegister("GET", "/api/v1/event-forward/logs", authz.Policy{Resource: "event_forward", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("GET", "/api/v1/event-forward/stats", authz.Policy{Resource: "event_forward", Action: "view", Scope: authz.ScopePlatform})
	reg.MustRegister("POST", "/api/v1/event-forward/stats/reset", authz.Policy{Resource: "event_forward", Action: "edit", Scope: authz.ScopePlatform})
}

func (m *Module) ensureHandler(host *module.Host) *efHandler.EventForwardHandler {
	if m.handler == nil {
		logger := host.Logger
		if logger == nil {
			logger = zap.NewNop()
		}
		m.handler = efHandler.NewEventForwardHandler(host.DB, logger)
	}
	return m.handler
}

func (m *Module) Start(ctx context.Context, host *module.Host) error {
	if host != nil && host.Config != nil {
		m.cfg = host.Config.ModuleSettings("eventforward")
	}
	h := m.ensureHandler(host)
	return h.StartWatchers()
}

func (m *Module) Stop(ctx context.Context) error {
	if m.handler != nil {
		m.handler.StopWatchers()
	}
	return nil
}

func (m *Module) failRateThreshold() float64 {
	if m.cfg.FailRateThreshold > 0 && m.cfg.FailRateThreshold <= 1 {
		return m.cfg.FailRateThreshold
	}
	return 0.9
}

func (m *Module) minMatched() int64 {
	if m.cfg.MinMatched > 0 {
		return m.cfg.MinMatched
	}
	return 20
}

func (m *Module) healthSustain() time.Duration {
	if m.cfg.HealthSustain == "" {
		return 2 * time.Minute
	}
	d, err := time.ParseDuration(m.cfg.HealthSustain)
	if err != nil || d <= 0 {
		return 2 * time.Minute
	}
	return d
}

func (m *Module) failRateDisabled() bool {
	return m.cfg.DisableFailRateCheck != nil && *m.cfg.DisableFailRateCheck
}

// failRateCondition returns (bad, failRate, message) without hysteresis.
func (m *Module) failRateCondition(s efHandler.StatsSnapshot) (bool, float64, string) {
	if m.failRateDisabled() {
		return false, 0, ""
	}
	minMatched := m.minMatched()
	threshold := m.failRateThreshold()
	total := s.ForwardOK + s.ForwardFail
	if s.EventsMatched < minMatched || total == 0 {
		return false, 0, ""
	}
	failRate := float64(s.ForwardFail) / float64(total)
	if failRate < threshold {
		return false, failRate, ""
	}
	msg := fmt.Sprintf("forward fail_rate=%.0f%% (ok=%d fail=%d, threshold=%.0f%%)",
		failRate*100, s.ForwardOK, s.ForwardFail, threshold*100)
	return true, failRate, msg
}

func (m *Module) Health(ctx context.Context) error {
	if m.handler == nil {
		return fmt.Errorf("eventforward handler not started")
	}
	s := m.handler.Stats()
	enabled := m.handler.EnabledRuleCount()
	if enabled > 0 && s.WatchersActive == 0 {
		return fmt.Errorf("enabled rules=%d but no active watchers", enabled)
	}

	bad, _, msg := m.failRateCondition(s)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !bad {
		m.failSince = time.Time{}
		return nil
	}
	if m.failSince.IsZero() {
		m.failSince = time.Now()
	}
	sustain := m.healthSustain()
	if time.Since(m.failSince) < sustain {
		// Hysteresis: still healthy until sustained
		return nil
	}
	return fmt.Errorf("%s (sustained >= %s)", msg, sustain)
}

func (m *Module) StatusDetails(ctx context.Context) map[string]any {
	if m.handler == nil {
		return nil
	}
	s := m.handler.Stats()
	enabled := m.handler.EnabledRuleCount()
	total := s.ForwardOK + s.ForwardFail
	failRate := 0.0
	if total > 0 {
		failRate = float64(s.ForwardFail) / float64(total)
	}

	details := map[string]any{
		"watchers_active":      s.WatchersActive,
		"enabled_rules":        enabled,
		"events_seen":          s.EventsSeen,
		"events_matched":       s.EventsMatched,
		"forward_ok":           s.ForwardOK,
		"forward_fail":         s.ForwardFail,
		"fail_rate":            failRate,
		"fail_rate_threshold":  m.failRateThreshold(),
		"min_matched":          m.minMatched(),
		"health_sustain":       m.healthSustain().String(),
		"fail_rate_check_off":  m.failRateDisabled(),
	}

	bad, _, msg := m.failRateCondition(s)
	m.mu.Lock()
	if !bad {
		m.failSince = time.Time{}
	}
	failSince := m.failSince
	m.mu.Unlock()
	if bad {
		sustain := m.healthSustain()
		elapsed := time.Duration(0)
		if !failSince.IsZero() {
			elapsed = time.Since(failSince)
		}
		if elapsed < sustain {
			details["health_warning"] = fmt.Sprintf("%s — 需持续 %s 后才标为不健康 (已 %.0fs)",
				msg, sustain, elapsed.Seconds())
			details["fail_since"] = failSince.Format(time.RFC3339)
		} else {
			details["health_warning"] = msg
		}
	}
	return details
}

func (m *Module) RegisterRoutes(ctx *module.Context, protected *gin.RouterGroup) {
	h := m.ensureHandler(ctx.Host)
	g := protected.Group("/event-forward")
	{
		g.GET("/rules", h.ListRules)
		g.POST("/rules", h.CreateRule)
		g.GET("/rules/:id", h.GetRule)
		g.PUT("/rules/:id", h.UpdateRule)
		g.DELETE("/rules/:id", h.DeleteRule)
		g.POST("/rules/:id/test", h.TestRule)
		g.GET("/logs", h.ListLogs)
		g.GET("/stats", h.GetStats)
		g.POST("/stats/reset", h.ResetStats)
	}
}
