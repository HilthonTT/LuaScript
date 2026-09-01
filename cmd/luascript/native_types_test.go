package main

import (
	"sort"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/typecheck"
	"github.com/hilthontt/luascript/internal/vm"
)

func callableWord(isFn bool) string {
	if isFn {
		return "a function"
	}
	return "a plain value"
}

func loadModuleValues(name string) (map[string]vm.Value, bool) {
	v := vm.New()
	registerAllNatives(v)

	pkg, ok := v.Globals.Get("package").(*vm.Table)
	if !ok {
		return nil, false
	}
	preload, ok := pkg.Get("preload").(*vm.Table)
	if !ok {
		return nil, false
	}
	loader := preload.Get(name)
	if loader == nil {
		return nil, false
	}
	res, _, failed := v.SafeCall(loader, []vm.Value{name})
	if failed || len(res) == 0 {
		return nil, false
	}
	mod, ok := res[0].(*vm.Table)
	if !ok {
		return nil, false
	}
	out := map[string]vm.Value{}
	collect := func(tb *vm.Table) {
		var k vm.Value
		for {
			var val vm.Value
			k, val = tb.Next(k)
			if k == nil {
				return
			}
			if s, ok := k.(string); ok {
				out[s] = val
			}
		}
	}
	collect(mod)
	if mt := mod.Metatable(); mt != nil {
		if idx, ok := mt.Get("__index").(*vm.Table); ok {
			collect(idx)
		}
	}
	return out, true
}

func TestNativeTypesMatchRuntime(t *testing.T) {
	names := typecheck.NativeModuleNames()
	sort.Strings(names)

	for _, name := range names {
		declared, ok := typecheck.NativeModuleFields(name)
		if !ok {
			t.Errorf("%s: listed by NativeModuleNames but has no declared fields", name)
			continue
		}
		live, ok := loadModuleMembers(name)
		if !ok {
			t.Errorf("%s: typed in native_types.go but no such module loads at runtime", name)
			continue
		}

		values, _ := loadModuleValues(name)
		for field := range declared {
			if !live[field] {
				t.Errorf("%s.%s: declared in native_types.go but absent at runtime — "+
					"a script using it would be rejected at compile time", name, field)
				continue
			}
			if wantFn, ok := typecheck.NativeFieldIsFunction(name, field); ok {
				_, isFn := values[field].(*vm.GoFunc)
				if _, isClosure := values[field].(*vm.Closure); isClosure {
					isFn = true
				}
				if values[field] != nil && wantFn != isFn {
					t.Errorf("%s.%s: declared as %s but is %s at runtime",
						name, field, callableWord(wantFn), callableWord(isFn))
				}
			}
		}
		for field := range live {
			if declared[field] || skipInAudit(field) {
				continue
			}
			t.Errorf("%s.%s: exists at runtime but is undeclared in native_types.go — "+
				"add it, or scripts using it will not type-check", name, field)
		}
	}
}

func TestNativeTypesCoverExpectedModules(t *testing.T) {
	names := typecheck.NativeModuleNames()
	if len(names) < 10 {
		t.Fatalf("only %d modules typed (%v); native_types.go looks truncated", len(names), names)
	}
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, want := range []string{"json", "crypto", "http", "os", "utf8", "time"} {
		if !have[want] {
			t.Errorf("module %q is not typed in native_types.go", want)
		}
	}
}
