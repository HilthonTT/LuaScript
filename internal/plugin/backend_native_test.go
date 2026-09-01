//go:build (linux || darwin || freebsd) && cgo

package plugin

import (
	"os/exec"
	"reflect"
	"testing"

	"github.com/hilthontt/luascript/internal/vm"
)

func TestBuildAndCallPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a Go plugin; skipped under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	t.Setenv("LUASCRIPT_PLUGIN_DIR", t.TempDir())

	s := &spec{
		Packages: []pkg{{Name: "strings"}},
		Functions: []function{
			{Pkg: "strings", Name: "ToUpper", As: "ToUpper"},
			{Pkg: "strings", Name: "Split", As: "Split"},
			{Pkg: "strings", Name: "NewReplacer", As: "NewReplacer"},
		},
	}

	so, err := buildPlugin("strutil", s)
	if err != nil {
		t.Fatalf("buildPlugin: %v", err)
	}
	lp, err := openPlugin(so)
	if err != nil {
		t.Fatalf("openPlugin: %v", err)
	}

	fn, err := lp.lookup("ToUpper")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if fn.Kind() != reflect.Func {
		t.Fatalf("ToUpper resolved to %s, not a func — pointer-to-var was not dereferenced", fn.Kind())
	}
	if got := callReflected(fn, "ToUpper", []vm.Value{"hello"}); got[0] != "HELLO" {
		t.Fatalf("ToUpper(\"hello\") = %#v, want \"HELLO\"", got[0])
	}

	split, _ := lp.lookup("Split")
	got := callReflected(split, "Split", []vm.Value{"a,b,c", ","})
	parts, ok := got[0].(*vm.Table)
	if !ok || parts.Len() != 3 || parts.Get(int64(2)) != "b" {
		t.Fatalf("Split = %#v, want a 3-element table", got[0])
	}

	mk, _ := lp.lookup("NewReplacer")
	rep := callReflected(mk, "NewReplacer", []vm.Value{"a", "1"})[0]
	self, ok := rep.(*vm.Table)
	if !ok {
		t.Fatalf("NewReplacer returned %T, want a GoValue table", rep)
	}
	raw, ok := unwrapGo(self)
	if !ok {
		t.Fatal("NewReplacer result is not a GoValue")
	}
	m, ok := goValueMember(self, raw, "Replace").(*vm.GoFunc)
	if !ok {
		t.Fatal("Replace did not resolve to a callable on the GoValue")
	}
	if out := m.Fn(nil, []vm.Value{self, "abc"}); out[0] != "1bc" {
		t.Fatalf("rep:Replace(\"abc\") = %#v, want \"1bc\"", out[0])
	}
}

func TestBuildPluginIsCached(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a Go plugin; skipped under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	t.Setenv("LUASCRIPT_PLUGIN_DIR", t.TempDir())

	s := &spec{
		Packages:  []pkg{{Name: "strings"}},
		Functions: []function{{Pkg: "strings", Name: "ToLower", As: "ToLower"}},
	}

	first, err := buildPlugin("lower", s)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, err := buildPlugin("lower", s)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first != second {
		t.Fatalf("cache miss: %q then %q", first, second)
	}
}
