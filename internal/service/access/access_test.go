package access

import (
	"reflect"
	"testing"
)

func TestEffectiveAccessMergesHighestPermission(t *testing.T) {
	effective := NewEffectiveAccess([]Grant{
		{ClusterID: 2, Namespace: "team-a", PermissionLevel: PermissionRead},
		{ClusterID: 2, Namespace: "team-a", PermissionLevel: PermissionAdmin},
		{ClusterID: 2, Namespace: "team-b", PermissionLevel: PermissionWrite},
		{ClusterID: 1, Namespace: "", PermissionLevel: PermissionWrite},
		{ClusterID: 3, Namespace: "ignored", PermissionLevel: "invalid"},
	})

	want := []Grant{
		{ClusterID: 1, Namespace: "*", PermissionLevel: PermissionWrite},
		{ClusterID: 2, Namespace: "team-a", PermissionLevel: PermissionAdmin},
		{ClusterID: 2, Namespace: "team-b", PermissionLevel: PermissionWrite},
	}
	if got := effective.Grants(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Grants() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAccessChecksClusterAndNamespaceLevels(t *testing.T) {
	effective := NewEffectiveAccess([]Grant{
		{ClusterID: 1, Namespace: "*", PermissionLevel: PermissionWrite},
		{ClusterID: 2, Namespace: "team-a", PermissionLevel: PermissionAdmin},
		{ClusterID: 2, Namespace: "team-b", PermissionLevel: PermissionRead},
	})

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{"wildcard permits namespaced read", effective.CanNamespace(1, "any", PermissionRead), true},
		{"write does not permit admin", effective.CanNamespace(1, "any", PermissionAdmin), false},
		{"scoped grant permits exact namespace", effective.CanNamespace(2, "team-a", PermissionWrite), true},
		{"scoped grant does not permit another namespace", effective.CanNamespace(2, "team-c", PermissionRead), false},
		{"scoped grant does not permit cluster resources", effective.CanCluster(2, PermissionRead), false},
		{"empty namespace is rejected", effective.CanNamespace(2, "", PermissionRead), false},
		{"invalid required level is rejected", effective.CanNamespace(2, "team-a", "owner"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("permission check = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestEffectiveAccessListsAccessibleScopes(t *testing.T) {
	effective := NewEffectiveAccess([]Grant{
		{ClusterID: 3, Namespace: "z", PermissionLevel: PermissionRead},
		{ClusterID: 1, Namespace: "*", PermissionLevel: PermissionWrite},
		{ClusterID: 3, Namespace: "a", PermissionLevel: PermissionAdmin},
		{ClusterID: 2, Namespace: "team", PermissionLevel: PermissionRead},
	})

	if got, want := effective.ClusterIDs(PermissionWrite), []uint{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ClusterIDs(write) = %#v, want %#v", got, want)
	}
	if got, want := effective.Namespaces(1, PermissionRead), []string{"*"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Namespaces(1, read) = %#v, want %#v", got, want)
	}
	if got, want := effective.Namespaces(3, PermissionRead), []string{"a", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Namespaces(3, read) = %#v, want %#v", got, want)
	}
	if got := effective.ClusterIDs("owner"); len(got) != 0 {
		t.Fatalf("ClusterIDs(invalid) = %#v, want empty", got)
	}
}
