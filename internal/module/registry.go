package module

import (
	"context"
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EnabledFunc reports whether a module should be active. Missing config => true.
type EnabledFunc func(name string) bool

// Registry holds registered modules and runs lifecycle hooks for enabled ones.
type Registry struct {
	enabled EnabledFunc
	mods    map[string]Module
	order   []string // topologically sorted enabled names after Resolve
	started []string
	logger  *zap.Logger
}

func NewRegistry(enabled EnabledFunc, logger *zap.Logger) *Registry {
	if enabled == nil {
		enabled = func(string) bool { return true }
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Registry{
		enabled: enabled,
		mods:    make(map[string]Module),
		logger:  logger,
	}
}

func (r *Registry) Register(m Module) error {
	meta := m.Meta()
	if meta.Name == "" {
		return fmt.Errorf("module name is required")
	}
	if _, exists := r.mods[meta.Name]; exists {
		return fmt.Errorf("module %q already registered", meta.Name)
	}
	r.mods[meta.Name] = m
	return nil
}

func (r *Registry) MustRegister(m Module) {
	if err := r.Register(m); err != nil {
		panic(err)
	}
}

func (r *Registry) Enabled(name string) bool {
	return r.enabled(name)
}

func (r *Registry) Get(name string) (Module, bool) {
	m, ok := r.mods[name]
	return m, ok
}

// ResolveEnabled returns enabled modules in dependency order.
func (r *Registry) ResolveEnabled() ([]Module, error) {
	enabled := make(map[string]Module)
	for name, m := range r.mods {
		if r.enabled(name) {
			enabled[name] = m
		}
	}
	ordered, err := topoSort(enabled)
	if err != nil {
		return nil, err
	}
	r.order = make([]string, 0, len(ordered))
	for _, m := range ordered {
		r.order = append(r.order, m.Meta().Name)
	}
	return ordered, nil
}

func topoSort(mods map[string]Module) ([]Module, error) {
	inDegree := make(map[string]int, len(mods))
	deps := make(map[string][]string, len(mods))
	names := make([]string, 0, len(mods))
	for name := range mods {
		names = append(names, name)
		inDegree[name] = 0
	}
	sort.Strings(names)
	for _, name := range names {
		m := mods[name]
		for _, dep := range m.Meta().Dependencies {
			if _, ok := mods[dep]; !ok {
				return nil, fmt.Errorf("module %q depends on disabled or missing module %q", name, dep)
			}
			deps[dep] = append(deps[dep], name)
			inDegree[name]++
		}
	}
	queue := make([]string, 0)
	for _, name := range names {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)
	out := make([]Module, 0, len(mods))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		out = append(out, mods[n])
		next := append([]string(nil), deps[n]...)
		sort.Strings(next)
		for _, child := range next {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
				sort.Strings(queue)
			}
		}
	}
	if len(out) != len(mods) {
		return nil, fmt.Errorf("module dependency cycle detected")
	}
	return out, nil
}

func (r *Registry) Migrate(db *gorm.DB) error {
	mods, err := r.ResolveEnabled()
	if err != nil {
		return err
	}
	models := make([]any, 0)
	for _, m := range mods {
		models = append(models, m.Migrations()...)
	}
	if len(models) == 0 {
		return nil
	}
	return db.AutoMigrate(models...)
}

func (r *Registry) RegisterPolicies(reg *authz.Registry) error {
	mods, err := r.ResolveEnabled()
	if err != nil {
		return err
	}
	for _, m := range mods {
		m.RegisterPolicies(reg)
	}
	return nil
}

func (r *Registry) RegisterRoutes(ctx *Context, protected *gin.RouterGroup) error {
	mods, err := r.ResolveEnabled()
	if err != nil {
		return err
	}
	for _, m := range mods {
		m.RegisterRoutes(ctx, protected)
	}
	return nil
}

func (r *Registry) Start(ctx context.Context, host *Host) error {
	mods, err := r.ResolveEnabled()
	if err != nil {
		return err
	}
	r.started = r.started[:0]
	for _, m := range mods {
		name := m.Meta().Name
		if err := m.Start(ctx, host); err != nil {
			_ = r.Stop(context.Background())
			return fmt.Errorf("start module %s: %w", name, err)
		}
		r.started = append(r.started, name)
		r.logger.Info("module started", zap.String("module", name))
	}
	return nil
}

func (r *Registry) Stop(ctx context.Context) error {
	var firstErr error
	for i := len(r.started) - 1; i >= 0; i-- {
		name := r.started[i]
		m := r.mods[name]
		if err := m.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("stop module %s: %w", name, err)
		}
	}
	r.started = nil
	return firstErr
}

func (r *Registry) statusOf(ctx context.Context, name string, m Module) Status {
	meta := m.Meta()
	st := Status{
		Name:          meta.Name,
		Version:       meta.Version,
		Description:   meta.Description,
		Enabled:       r.enabled(name),
		MultiInstance: meta.MultiInstance,
		Healthy:       true,
	}
	if st.Enabled {
		if err := m.Health(ctx); err != nil {
			st.Healthy = false
			st.HealthError = err.Error()
		}
		if d, ok := m.(StatusDetailer); ok {
			st.Details = d.StatusDetails(ctx)
		}
	}
	return st
}

func (r *Registry) Status(ctx context.Context) []Status {
	names := make([]string, 0, len(r.mods))
	for name := range r.mods {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Status, 0, len(names))
	for _, name := range names {
		out = append(out, r.statusOf(ctx, name, r.mods[name]))
	}
	return out
}

// StatusOne returns status for a single registered module.
func (r *Registry) StatusOne(ctx context.Context, name string) (Status, bool) {
	m, ok := r.mods[name]
	if !ok {
		return Status{}, false
	}
	return r.statusOf(ctx, name, m), true
}

func (r *Registry) Menus() []MenuItem {
	mods, err := r.ResolveEnabled()
	if err != nil {
		return nil
	}
	items := make([]MenuItem, 0)
	for _, m := range mods {
		items = append(items, m.Menus()...)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Order == items[j].Order {
			return items[i].Key < items[j].Key
		}
		return items[i].Order < items[j].Order
	})
	return items
}
