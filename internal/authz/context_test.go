package authz

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAllowedNamespaceSetAndFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	if AllowedNamespaceSet(c) != nil {
		t.Fatal("missing key should mean unrestricted")
	}
	c.Set(AllowedNamespacesKey, []string{"*"})
	if AllowedNamespaceSet(c) != nil {
		t.Fatal("wildcard should mean unrestricted")
	}

	c.Set(AllowedNamespacesKey, []string{"dev", "qa"})
	set := AllowedNamespaceSet(c)
	if len(set) != 2 {
		t.Fatalf("expected 2 namespaces, got %#v", set)
	}
	if !NamespaceAllowed(c, "dev") || NamespaceAllowed(c, "prod") {
		t.Fatal("namespace allow check failed")
	}
	filtered := FilterNamespaces(c, []string{"dev", "prod", "qa"})
	if len(filtered) != 2 || filtered[0] != "dev" || filtered[1] != "qa" {
		t.Fatalf("unexpected filter result %#v", filtered)
	}
}
