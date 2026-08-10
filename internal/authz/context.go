package authz

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
)

const ContextAuthorizerKey = "authz_authorizer"

// FromContext returns the request-scoped Authorizer when available.
func FromContext(c *gin.Context) (*Authorizer, bool) {
	raw, ok := c.Get(ContextAuthorizerKey)
	if !ok {
		return nil, false
	}
	authorizer, ok := raw.(*Authorizer)
	return authorizer, ok
}

// RequireScope re-checks platform RBAC plus cluster/namespace grants for
// handler-level multi-target and AI execution paths.
func RequireScope(c *gin.Context, resource, action string, clusterID uint, namespace string) error {
	authorizer, ok := FromContext(c)
	if !ok {
		return fmt.Errorf("authorizer is not configured")
	}
	userID, roleID, ok := principalIDs(c)
	if !ok {
		return fmt.Errorf("missing authentication context")
	}
	return authorizer.Check(c.Request.Context(), userID, roleID, resource, action, clusterID, namespace)
}

// EnsureScope writes a 403 response and returns false when scope authz fails.
func EnsureScope(c *gin.Context, resource, action string, clusterID uint, namespace string) bool {
	if err := RequireScope(c, resource, action, clusterID, namespace); err != nil {
		response.Forbidden(c, "cluster access denied")
		return false
	}
	return true
}

// AllowedNamespaceSet returns the caller-restricted namespace set.
// A nil map means unrestricted (cluster-wide / admin / "*").
func AllowedNamespaceSet(c *gin.Context) map[string]struct{} {
	raw, ok := c.Get(AllowedNamespacesKey)
	if !ok {
		return nil
	}
	allowed, ok := raw.([]string)
	if !ok || len(allowed) == 0 {
		return nil
	}
	if len(allowed) == 1 && allowed[0] == "*" {
		return nil
	}
	set := make(map[string]struct{}, len(allowed))
	for _, ns := range allowed {
		if ns == "" || ns == "*" {
			continue
		}
		set[ns] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// NamespaceAllowed reports whether a concrete namespace is in the allowed set.
func NamespaceAllowed(c *gin.Context, namespace string) bool {
	set := AllowedNamespaceSet(c)
	if set == nil {
		return true
	}
	_, ok := set[namespace]
	return ok
}
