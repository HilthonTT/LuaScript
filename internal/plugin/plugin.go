// Package plugin lets a LuaScript program reach into arbitrary Go libraries
// at run time, in the spirit of Goby's native/plugin.
//
// A script declares which Go packages it wants and which functions to lift out
// of them. The module generates a small `package main` that re-exports those
// functions as package-level vars, compiles it with `go build -buildmode=plugin`,
// opens the resulting shared object, and dispatches calls through reflect:
//
//	local plugin = require("plugin")
//
//	local p = plugin.generate("strutil", {
//	  packages  = { { name = "strings" } },
//	  functions = { { pkg = "strings", name = "ToUpper" } },
//	})
//
//	print(p:call("ToUpper", "hello"))  --> HELLO
//	print(p.ToUpper("hello"))          --> HELLO  (same call, via __index)
//
// Go values with no Lua counterpart — a *sql.DB, say — come back as GoValue
// objects whose exported methods and fields stay reachable (see convert.go),
// so the database/sql idiom Goby demonstrates works here too:
//
//	local p = plugin.generate("db", {
//	  packages  = { { name = "database/sql" }, { prefix = "_", name = "github.com/lib/pq" } },
//	  functions = { { pkg = "sql", name = "Open" } },
//	})
//	local db, err = p.Open("postgres", "...")
//	local rows    = db:Query("select 1")
//
// # Platform support
//
// Go's plugin package only exists on linux, darwin and freebsd, and only with
// cgo. On Windows there is no dynamic loading to be had — plugin.Open returns
// "plugin: not implemented" — so this module still loads (require succeeds) but
// generate/open raise an error naming the platform. Scripts that want to stay
// portable should branch on `plugin.supported` first.
//
// # Security
//
// generate() runs the Go compiler and then loads native code into this process.
// That is arbitrary code execution by construction — the same bargain Goby
// makes, and the whole point of the feature. Do not hand a spec built from
// untrusted input to this module.
package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterPluginPreload installs the `plugin` module under package.preload.
func RegisterPluginPreload(v *vm.VM) {
	vm.RegisterPreload(v, "plugin", pluginLoader)
}

func pluginLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 5)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		m.Set(name, &vm.GoFunc{Name: "plugin." + name, Fn: fn})
	}

	// Feature detection, so a portable script can degrade instead of dying.
	m.Set("supported", pluginSupported)
	set("unsupported_reason", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if pluginSupported {
			return []vm.Value{nil}
		}
		return []vm.Value{unsupportedReason()}
	})

	set("dir", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{pluginDir()}
	})

	set("generate", func(_ *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("plugin.generate", 1, args)
		s := parseSpec(vm.TableArg("plugin.generate", 2, args))

		so, err := buildPlugin(name, s)
		if err != nil {
			panic(vm.Errorf("plugin.generate: %v", err))
		}
		lp, err := openPlugin(so)
		if err != nil {
			panic(vm.Errorf("plugin.generate: %v", err))
		}
		return []vm.Value{wrapPlugin(lp)}
	})

	set("open", func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("plugin.open", 1, args)
		lp, err := openPlugin(path)
		if err != nil {
			panic(vm.Errorf("plugin.open: %v", err))
		}
		return []vm.Value{wrapPlugin(lp)}
	})

	return []vm.Value{m}
}

// parseSpec reads the Lua spec table:
//
//	{
//	  packages  = { { name = "database/sql" }, { prefix = "_", name = "github.com/lib/pq" } },
//	  functions = { { pkg = "sql", name = "Open", as = "OpenDB" } },
//	}
//
// `prefix` defaults to "" (import under the package's own name) and `as`
// defaults to `name`. Everything is validated in spec.validate before it is
// interpolated into Go source.
func parseSpec(t *vm.Table) *spec {
	s := &spec{}

	pkgs, _ := t.Get("packages").(*vm.Table)
	if pkgs == nil {
		panic(vm.Errorf("plugin spec: `packages` must be a table of { prefix = ..., name = ... }"))
	}
	for i := int64(1); i <= pkgs.Len(); i++ {
		e, ok := pkgs.Get(i).(*vm.Table)
		if !ok {
			panic(vm.Errorf("plugin spec: packages[%d] must be a table", i))
		}
		name, ok := e.Get("name").(string)
		if !ok {
			panic(vm.Errorf("plugin spec: packages[%d].name must be a string", i))
		}
		prefix, _ := e.Get("prefix").(string)
		s.Packages = append(s.Packages, pkg{Prefix: prefix, Name: name})
	}

	fns, _ := t.Get("functions").(*vm.Table)
	if fns == nil {
		panic(vm.Errorf("plugin spec: `functions` must be a table of { pkg = ..., name = ... }"))
	}
	for i := int64(1); i <= fns.Len(); i++ {
		e, ok := fns.Get(i).(*vm.Table)
		if !ok {
			panic(vm.Errorf("plugin spec: functions[%d] must be a table", i))
		}
		name, ok := e.Get("name").(string)
		if !ok {
			panic(vm.Errorf("plugin spec: functions[%d].name must be a string", i))
		}
		pkgSel, ok := e.Get("pkg").(string)
		if !ok {
			panic(vm.Errorf("plugin spec: functions[%d].pkg must be a string (the package selector, e.g. \"sql\")", i))
		}
		as, _ := e.Get("as").(string)
		if as == "" {
			as = name
		}
		s.Functions = append(s.Functions, function{Pkg: pkgSel, Name: name, As: as})
	}

	if err := s.validate(); err != nil {
		panic(vm.Errorf("plugin spec: %v", err))
	}
	return s
}

// loadedKey is the private instance-table key holding the *loadedPlugin.
const loadedKey = "\x00plugin"

// wrapPlugin exposes an opened plugin to Lua. Symbols are resolved lazily by
// __index — `p.ToUpper` looks ToUpper up in the .so on first use and caches
// the resulting callable on the table, so later reads are plain table hits.
func wrapPlugin(lp *loadedPlugin) *vm.Table {
	t := vm.NewTable(0, 2)
	t.Set(loadedKey, lp)

	// p:call("Name", ...) — the explicit form, and the one that gives a clear
	// error for a symbol that isn't there.
	t.Set("call", &vm.GoFunc{Name: "plugin:call", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) > 0 {
			if self, ok := args[0].(*vm.Table); ok && self == t {
				args = args[1:]
			}
		}
		name := vm.StringArg("plugin:call", 1, args)
		fn, err := lp.lookup(name)
		if err != nil {
			panic(vm.Errorf("plugin:call: %v", err))
		}
		if fn.Kind() != reflect.Func {
			panic(vm.Errorf("plugin:call: symbol %q is a %s, not a function", name, fn.Kind()))
		}
		return callReflected(fn, name, args[1:])
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", &vm.GoFunc{Name: "plugin:__index", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 2 {
			return []vm.Value{nil}
		}
		self, _ := args[0].(*vm.Table)
		key, ok := args[1].(string)
		if !ok || self == nil {
			return []vm.Value{nil}
		}

		rv, err := lp.lookup(key)
		if err != nil {
			// A missing symbol reads as nil rather than raising, so
			// `if p.Maybe then` stays a legal thing to write. p:call is
			// the form that reports the error.
			return []vm.Value{nil}
		}

		var out vm.Value
		if rv.Kind() == reflect.Func {
			out = &vm.GoFunc{Name: "plugin." + key, Fn: func(_ *vm.VM, callArgs []vm.Value) []vm.Value {
				return callReflected(rv, key, callArgs)
			}}
		} else {
			out = fromGo(rv)
		}
		self.Set(key, out) // memoize: subsequent reads never reach __index
		return []vm.Value{out}
	}})
	t.SetMetatable(mt)

	return t
}

// pluginDir is the directory generated sources and compiled .so files are
// written to. It follows the same convention as the bytecode cache
// (internal/compiler/bccache): under the user cache dir by default, relocatable
// with LUASCRIPT_PLUGIN_DIR — which is also how the tests keep their artifacts
// out of the real cache.
func pluginDir() string {
	if d := os.Getenv("LUASCRIPT_PLUGIN_DIR"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "luascript", "plugins")
	}
	return filepath.Join(base, "luascript", "plugins")
}

// sourceHash keys a build directory by its generated source, so an unchanged
// spec is a cache hit and a changed one cannot collide with the old artifact.
func sourceHash(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:8])
}
