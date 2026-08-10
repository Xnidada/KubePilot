package authz

import (
	"context"

	"github.com/kubepilot/kubepilot/internal/service/access"
)

// AccessServiceResolver adapts the effective access service to GrantResolver.
type AccessServiceResolver struct {
	svc *access.Service
}

func NewAccessServiceResolver(svc *access.Service) *AccessServiceResolver {
	return &AccessServiceResolver{svc: svc}
}

func (r *AccessServiceResolver) Authorize(ctx context.Context, userID, clusterID uint, namespace, requiredLevel string) (bool, error) {
	level := access.PermissionLevel(requiredLevel)
	if namespace == "*" {
		return r.svc.CanCluster(ctx, userID, clusterID, level)
	}
	return r.svc.CanNamespace(ctx, userID, clusterID, namespace, level)
}

func (r *AccessServiceResolver) AllowedNamespaces(ctx context.Context, userID, clusterID uint, requiredLevel string) ([]string, error) {
	return r.svc.Namespaces(ctx, userID, clusterID, access.PermissionLevel(requiredLevel))
}
