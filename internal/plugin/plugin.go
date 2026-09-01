package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"

	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterPluginPreload(v *vm.VM) {
	vm.RegisterPreload(v, "plugin", pluginLoader)
}

func pluginLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 5)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		m.Set(name, &vm.GoFunc{Name: "plugin." + name, Fn: fn})
	}

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

const loadedKey = "\x00plugin"

func wrapPlugin(lp *loadedPlugin) *vm.Table {
	t := vm.NewTable(0, 2)
	t.Set(loadedKey, lp)

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
		self.Set(key, out)
		return []vm.Value{out}
	}})
	t.SetMetatable(mt)

	return t
}

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

func sourceHash(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:8])
}
