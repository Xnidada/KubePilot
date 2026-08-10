package authz

import (
	"net/http"
	"testing"
)

func TestRegistryRejectsDuplicatesAndMissingFields(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(http.MethodGet, "/api/v1/clusters", Policy{Resource: "clusters", Action: "view", Scope: ScopePlatform}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registry.Register(http.MethodGet, "/api/v1/clusters", Policy{Resource: "clusters", Action: "view", Scope: ScopePlatform}); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if err := registry.Register(http.MethodPost, "/api/v1/x", Policy{}); err == nil {
		t.Fatal("expected missing resource/action error")
	}
}

func TestRequiredLevelMapping(t *testing.T) {
	if RequiredLevel("view") != "read" {
		t.Fatalf("view => read")
	}
	if RequiredLevel("exec") != "write" {
		t.Fatalf("exec => write")
	}
	if RequiredLevel("admin") != "admin" {
		t.Fatalf("admin => admin")
	}
}

func TestPathScopeResolverRequiresNamespace(t *testing.T) {
	resolver := PathScopeResolver{}
	_, err := resolver.Resolve(nil, Policy{Scope: ScopeHandler})
	if err == nil {
		t.Fatal("handler scope should fail in path resolver")
	}
}
