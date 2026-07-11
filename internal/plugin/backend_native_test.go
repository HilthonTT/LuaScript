//go:build (linux || darwin || freebsd) && cgo

package plugin

import (
	"os/exec"
	"reflect"
	"testing"

	"github.com/hilthontt/luascript/internal/vm"
)

// End-to-end: generate a plugin over the standard library, compile it with
// `go build -buildmode=plugin`, load it, and call through it. This is the only
// test that proves the whole chain — and it cannot run on Windows, which is why
// everything else in this package is written to be platform-independent.
//
// Only the standard library is used, so no network round-trip is needed.
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

	// A generated `var ToUpper = strings.ToUpper` is looked up as a *pointer*
	// to the var; lookup has to dereference it before it is callable.
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

	// A Go slice return crosses back as a Lua table.
	split, _ := lp.lookup("Split")
	got := callReflected(split, "Split", []vm.Value{"a,b,c", ","})
	parts, ok := got[0].(*vm.Table)
	if !ok || parts.Len() != 3 || parts.Get(int64(2)) != "b" {
		t.Fatalf("Split = %#v, want a 3-element table", got[0])
	}

	// A *strings.Replacer has no Lua counterpart: it comes back as a GoValue
	// whose methods are still callable, and it can be passed back into Go.
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

// The second build of an unchanged spec must be a cache hit rather than
// another trip through the compiler.
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
