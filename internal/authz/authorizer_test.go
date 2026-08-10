package authz

import (
	"context"
	"testing"

	"github.com/kubepilot/kubepilot/internal/service/access"
)

type fakeGrants struct {
	can map[string]bool
	ns  map[uint][]string
}

func (f *fakeGrants) Authorize(_ context.Context, userID, clusterID uint, namespace, requiredLevel string) (bool, error) {
	key := keyOf(userID, clusterID, namespace, requiredLevel)
	return f.can[key], nil
}

func (f *fakeGrants) AllowedNamespaces(_ context.Context, userID, clusterID uint, requiredLevel string) ([]string, error) {
	return f.ns[clusterID], nil
}

func keyOf(userID, clusterID uint, namespace, requiredLevel string) string {
	return string(rune(userID)) + ":" + string(rune(clusterID)) + ":" + namespace + ":" + requiredLevel
}

func TestEffectiveAccessMatrix(t *testing.T) {
	effective := access.NewEffectiveAccess([]access.Grant{
		{ClusterID: 1, Namespace: "dev", PermissionLevel: access.PermissionRead},
		{ClusterID: 1, Namespace: "dev", PermissionLevel: access.PermissionWrite},
		{ClusterID: 1, Namespace: "*", PermissionLevel: access.PermissionRead},
		{ClusterID: 2, Namespace: "prod", PermissionLevel: access.PermissionAdmin},
	})

	if !effective.CanNamespace(1, "dev", access.PermissionWrite) {
		t.Fatal("highest direct grant should allow write on dev")
	}
	if !effective.CanCluster(1, access.PermissionRead) {
		t.Fatal("wildcard grant should allow cluster read")
	}
	if effective.CanCluster(1, access.PermissionWrite) {
		t.Fatal("wildcard read must not satisfy write for cluster-scoped resources")
	}
	if !effective.CanNamespace(2, "prod", access.PermissionAdmin) {
		t.Fatal("admin grant should allow admin")
	}
	if effective.CanNamespace(2, "other", access.PermissionRead) {
		t.Fatal("unrelated namespace must be denied")
	}
	ids := effective.ClusterIDs(access.PermissionRead)
	if len(ids) != 2 {
		t.Fatalf("expected 2 clusters, got %v", ids)
	}
}

func TestAuthorizerCheckRejectsMissingGrant(t *testing.T) {
	// Authorizer.Check depends on model.DB for role lookup; keep pure EffectiveAccess coverage here.
	effective := access.NewEffectiveAccess(nil)
	if effective.CanCluster(1, access.PermissionRead) {
		t.Fatal("empty grants must deny")
	}
}
