package module

import (
	"testing"

	"github.com/gin-gonic/gin"
)

type testMod struct {
	Base
	name string
	deps []string
}

func (m testMod) Meta() Metadata {
	return Metadata{Name: m.name, Version: "1", Dependencies: m.deps, MultiInstance: MultiInstanceAll}
}

func (m testMod) RegisterRoutes(ctx *Context, protected *gin.RouterGroup) {}

func TestTopoSortDependencies(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.MustRegister(testMod{name: "a"})
	r.MustRegister(testMod{name: "b", deps: []string{"a"}})
	r.MustRegister(testMod{name: "c", deps: []string{"b"}})
	mods, err := r.ResolveEnabled()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{mods[0].Meta().Name, mods[1].Meta().Name, mods[2].Meta().Name}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order=%v want=%v", got, want)
		}
	}
}

func TestDisabledDependencyFails(t *testing.T) {
	r := NewRegistry(func(name string) bool { return name != "a" }, nil)
	r.MustRegister(testMod{name: "a"})
	r.MustRegister(testMod{name: "b", deps: []string{"a"}})
	if _, err := r.ResolveEnabled(); err == nil {
		t.Fatal("expected dependency error")
	}
}

func TestModuleEnabledDefaultTrue(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.MustRegister(testMod{name: "x"})
	mods, err := r.ResolveEnabled()
	if err != nil || len(mods) != 1 {
		t.Fatalf("mods=%v err=%v", mods, err)
	}
}
