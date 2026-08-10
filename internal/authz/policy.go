package authz

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type Scope string

const (
	ScopePlatform        Scope = "platform"
	ScopeCluster         Scope = "cluster"
	ScopeNamespace       Scope = "namespace"
	ScopeHandler         Scope = "handler"
	ScopeNamespaceList   Scope = "namespace_list"
	AllowedNamespacesKey       = "authz_allowed_namespaces"
)

type Policy struct {
	Resource                    string
	Action                      string
	Scope                       Scope
	AuthenticatedOnly           bool
	AllowFilteredNamespaceList  bool
}

type Registry struct {
	mu       sync.RWMutex
	policies map[string]Policy
}

func NewRegistry() *Registry {
	return &Registry{policies: make(map[string]Policy)}
}

func Key(method, fullPath string) string {
	return strings.ToUpper(method) + " " + fullPath
}

func (r *Registry) Register(method, fullPath string, policy Policy) error {
	if method == "" || fullPath == "" {
		return fmt.Errorf("method and full path are required")
	}
	if !policy.AuthenticatedOnly && (policy.Resource == "" || policy.Action == "") {
		return fmt.Errorf("resource and action are required for %s", Key(method, fullPath))
	}
	key := Key(method, fullPath)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.policies[key]; exists {
		return fmt.Errorf("policy already registered for %s", key)
	}
	r.policies[key] = policy
	return nil
}

func (r *Registry) MustRegister(method, fullPath string, policy Policy) {
	if err := r.Register(method, fullPath, policy); err != nil {
		panic(err)
	}
}

func (r *Registry) Lookup(method, fullPath string) (Policy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy, ok := r.policies[Key(method, fullPath)]
	return policy, ok
}

func (r *Registry) Registered(method, fullPath string) bool {
	_, ok := r.Lookup(method, fullPath)
	return ok
}

// Keys returns all registered policy keys (METHOD path).
func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.policies))
	for key := range r.policies {
		keys = append(keys, key)
	}
	return keys
}

func RequiredLevel(action string) string {
	switch action {
	case "view":
		return "read"
	case "create", "edit", "delete", "execute", "exec":
		return "write"
	case "admin", "*":
		return "admin"
	default:
		return ""
	}
}

func MethodAction(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return "view"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "edit"
	case http.MethodDelete:
		return "delete"
	default:
		return ""
	}
}

type ScopeTarget struct {
	ClusterID uint
	Namespace string
}

type ScopeResolver interface {
	Resolve(*gin.Context, Policy) (ScopeTarget, error)
}

type PathScopeResolver struct{}

func (PathScopeResolver) Resolve(c *gin.Context, policy Policy) (ScopeTarget, error) {
	if policy.Scope == ScopePlatform {
		return ScopeTarget{}, nil
	}
	if policy.Scope == ScopeHandler {
		return ScopeTarget{}, fmt.Errorf("authorization scope requires a handler-level resolver")
	}

	clusterID, err := parseUint(c.Param("id"))
	if err != nil {
		return ScopeTarget{}, fmt.Errorf("invalid cluster id")
	}
	target := ScopeTarget{ClusterID: clusterID}
	if policy.Scope == ScopeCluster {
		target.Namespace = "*"
		return target, nil
	}

	target.Namespace = firstNonEmpty(c.Param("ns"), c.Query("namespace"), c.Query("ns"))
	if target.Namespace == "" {
		return ScopeTarget{}, fmt.Errorf("namespace is required")
	}
	return target, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func parseUint(value string) (uint, error) {
	var result uint64
	if _, err := fmt.Sscan(value, &result); err != nil || result == 0 {
		return 0, fmt.Errorf("invalid unsigned integer")
	}
	return uint(result), nil
}
