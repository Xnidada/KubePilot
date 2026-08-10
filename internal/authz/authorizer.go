package authz

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
)

const (
	ContextResourceKey  = "authz_resource"
	ContextActionKey    = "authz_action"
	ContextDecisionKey  = "authz_decision"
	ContextReasonKey    = "authz_reason"
	ContextClusterKey   = "authz_cluster_id"
	ContextNamespaceKey = "authz_namespace"
)

// Authorizer evaluates explicit route policies against platform RBAC and cluster grants.
type Authorizer struct {
	registry *Registry
	grants   GrantResolver
	resolver ScopeResolver
}

func NewAuthorizer(registry *Registry, grants GrantResolver, resolver ScopeResolver) *Authorizer {
	if resolver == nil {
		resolver = PathScopeResolver{}
	}
	return &Authorizer{registry: registry, grants: grants, resolver: resolver}
}

func (a *Authorizer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		policy, ok := a.registry.Lookup(c.Request.Method, c.FullPath())
		if !ok {
			setDecision(c, "", "", "deny", "route policy is not registered")
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		if policy.AuthenticatedOnly {
			setDecision(c, "self", "access", "allow", "authenticated self route")
			c.Next()
			return
		}

		userID, roleID, ok := principalIDs(c)
		if !ok {
			setDecision(c, policy.Resource, policy.Action, "deny", "missing authentication context")
			response.Forbidden(c, "missing user authorization context")
			c.Abort()
			return
		}

		var role model.Role
		if err := model.DB.WithContext(c.Request.Context()).First(&role, roleID).Error; err != nil {
			setDecision(c, policy.Resource, policy.Action, "deny", "role not found")
			response.Forbidden(c, "role not found")
			c.Abort()
			return
		}

		if role.IsSystem || role.Name == "admin" {
			setDecision(c, policy.Resource, policy.Action, "allow", "admin bypass")
			c.Next()
			return
		}

		permissions, err := model.ParsePermissions(role.Permissions)
		if err != nil {
			setDecision(c, policy.Resource, policy.Action, "deny", "invalid permissions")
			response.Forbidden(c, "invalid permissions")
			c.Abort()
			return
		}
		if !permissions.HasPermission(policy.Resource, policy.Action) {
			setDecision(c, policy.Resource, policy.Action, "deny", "platform permission denied")
			response.Forbidden(c, "insufficient permissions")
			c.Abort()
			return
		}

		if policy.Scope == ScopePlatform {
			setDecision(c, policy.Resource, policy.Action, "allow", "platform permission granted")
			c.Next()
			return
		}
		if policy.Scope == ScopeHandler {
			setDecision(c, policy.Resource, policy.Action, "allow", "handler-level scope deferred")
			c.Next()
			return
		}

		if err := a.authorizeScope(c, userID, policy); err != nil {
			setDecision(c, policy.Resource, policy.Action, "deny", err.Error())
			if err.Error() == "invalid cluster id" || err.Error() == "namespace is required" {
				response.BadRequest(c, err.Error())
			} else {
				response.Forbidden(c, "cluster access denied")
			}
			c.Abort()
			return
		}

		setDecision(c, policy.Resource, policy.Action, "allow", "scope permission granted")
		c.Next()
	}
}

func (a *Authorizer) authorizeScope(c *gin.Context, userID uint, policy Policy) error {
	level := RequiredLevel(policy.Action)
	if level == "" {
		return fmt.Errorf("unsupported action %q", policy.Action)
	}

	switch policy.Scope {
	case ScopeCluster:
		target, err := a.resolver.Resolve(c, policy)
		if err != nil {
			return err
		}
		c.Set(ContextClusterKey, target.ClusterID)
		c.Set(ContextNamespaceKey, "*")
		ok, err := a.grants.Authorize(c.Request.Context(), userID, target.ClusterID, "*", level)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cluster access denied")
		}
		return nil

	case ScopeNamespace:
		target, err := a.resolver.Resolve(c, policy)
		if err != nil {
			return err
		}
		c.Set(ContextClusterKey, target.ClusterID)
		c.Set(ContextNamespaceKey, target.Namespace)
		ok, err := a.grants.Authorize(c.Request.Context(), userID, target.ClusterID, target.Namespace, level)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cluster access denied")
		}
		return nil

	case ScopeNamespaceList:
		clusterID, err := parseUint(c.Param("id"))
		if err != nil {
			return fmt.Errorf("invalid cluster id")
		}
		c.Set(ContextClusterKey, clusterID)
		ns := firstNonEmpty(c.Param("ns"), c.Query("namespace"), c.Query("ns"))
		if ns != "" {
			c.Set(ContextNamespaceKey, ns)
			ok, err := a.grants.Authorize(c.Request.Context(), userID, clusterID, ns, level)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("cluster access denied")
			}
			return nil
		}

		// Fail closed for namespace-restricted users listing without an explicit ns.
		ok, err := a.grants.Authorize(c.Request.Context(), userID, clusterID, "*", level)
		if err != nil {
			return err
		}
		if ok {
			c.Set(ContextNamespaceKey, "*")
			c.Set(AllowedNamespacesKey, []string{"*"})
			return nil
		}
		namespaces, err := a.grants.AllowedNamespaces(c.Request.Context(), userID, clusterID, level)
		if err != nil {
			return err
		}
		filtered := make([]string, 0, len(namespaces))
		for _, item := range namespaces {
			if item != "" && item != "*" {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("cluster access denied")
		}
		if !policy.AllowFilteredNamespaceList {
			return fmt.Errorf("namespace is required")
		}
		// Namespace-restricted callers may list only their granted namespaces.
		c.Set(AllowedNamespacesKey, filtered)
		c.Set(ContextNamespaceKey, "")
		return nil

	default:
		return fmt.Errorf("unsupported authorization scope")
	}
}

// Check is a reusable helper for handlers and AI executors.
func (a *Authorizer) Check(ctx context.Context, userID, roleID uint, resource, action string, clusterID uint, namespace string) error {
	var role model.Role
	if err := model.DB.WithContext(ctx).First(&role, roleID).Error; err != nil {
		return fmt.Errorf("role not found")
	}
	if role.IsSystem || role.Name == "admin" {
		return nil
	}
	permissions, err := model.ParsePermissions(role.Permissions)
	if err != nil {
		return fmt.Errorf("invalid permissions")
	}
	if !permissions.HasPermission(resource, action) {
		return fmt.Errorf("insufficient permissions")
	}
	if clusterID == 0 {
		return nil
	}
	if namespace == "" {
		namespace = "*"
	}
	ok, err := a.grants.Authorize(ctx, userID, clusterID, namespace, RequiredLevel(action))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("cluster access denied")
	}
	return nil
}

func principalIDs(c *gin.Context) (uint, uint, bool) {
	userRaw, userOK := c.Get("user_id")
	roleRaw, roleOK := c.Get("role_id")
	if !userOK || !roleOK {
		return 0, 0, false
	}
	userID, userOK := asUint(userRaw)
	roleID, roleOK := asUint(roleRaw)
	return userID, roleID, userOK && roleOK
}

func asUint(value interface{}) (uint, bool) {
	switch v := value.(type) {
	case uint:
		return v, true
	case uint64:
		return uint(v), true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case string:
		parsed, err := strconv.ParseUint(v, 10, 32)
		return uint(parsed), err == nil
	default:
		return 0, false
	}
}

func setDecision(c *gin.Context, resource, action, decision, reason string) {
	if resource != "" {
		c.Set(ContextResourceKey, resource)
	}
	if action != "" {
		c.Set(ContextActionKey, action)
	}
	c.Set(ContextDecisionKey, decision)
	c.Set(ContextReasonKey, reason)
}


func (a *Authorizer) Authorize(ctx context.Context, userID, clusterID uint, namespace, requiredLevel string) (bool, error) {
	return a.grants.Authorize(ctx, userID, clusterID, namespace, requiredLevel)
}

func (a *Authorizer) AllowedNamespaces(ctx context.Context, userID, clusterID uint, requiredLevel string) ([]string, error) {
	return a.grants.AllowedNamespaces(ctx, userID, clusterID, requiredLevel)
}
