// Package enumrt provides the runtime glue the bytecode generator emits
// calls into when lowering an `enum` declaration. It installs ONE global
// helper, `__enum_freeze(table, name) -> table`, that:
//
//  1. attaches an `__newindex` metamethod which raises
//     "attempt to modify enum '<name>'" when anyone assigns into the
//     table (preserving the "enum values are constants" contract), and
//  2. sets `__metatable = "enum"` so that `setmetatable`/`getmetatable`
//     can't subvert (1) by swapping the metatable out.
//
// The double-underscore name is deliberately ugly — this is a compiler
// emit target, not a user-facing API. Adding a real user-facing
// `table.freeze` is a follow-up; the surface we need here is narrower.
//
// Registration mirrors every other native module (a single function
// pushed onto cmd/natives.go::nativeRegistrars), but enumrt installs
// the helper directly on Globals rather than under `package.preload` —
// the bytecode generator emits an unconditional `GetGlobal
// "__enum_freeze"`, so it must be present before any user code runs,
// not loaded on first `require`.
package enumrt

import (
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

// freezeGlobalName is the global slot the bytecode generator looks up
// when lowering an enum declaration. Kept as a named constant so a
// future rename touches both sites in one place.
const freezeGlobalName = "__enum_freeze"

// adtGlobalName is the emit target for a *tagged* (sum-type) enum. The
// bytecode generator calls `__enum_adt(name, arities)` where `arities`
// maps each variant name to its payload arity.
const adtGlobalName = "__enum_adt"

// RegisterEnumRT installs __enum_freeze and __enum_adt on the VM's globals.
// Idempotent — calling it twice replaces the bindings with the same
// functions, matching how the other registrars behave when the REPL replays
// them on :reset.
func RegisterEnumRT(v *vm.VM) {
	v.SetGlobal(freezeGlobalName, &vm.GoFunc{
		Name: freezeGlobalName,
		Fn:   freezeEnum,
	})
	v.SetGlobal(adtGlobalName, &vm.GoFunc{
		Name: adtGlobalName,
		Fn:   defineADT,
	})
}

// freezeEnum implements __enum_freeze. Args: (values, name) — the
// caller passes the populated variant→int table as `values`. We build
// and return an empty *proxy* whose metatable routes reads to `values`
// (__index) and rejects writes (__newindex).
//
// The proxy indirection is what makes the freeze actually work: Lua's
// __newindex only fires when the key is absent from the visible table.
// If we attached __newindex to the populated table directly, writes
// like `Color.RED = 99` (existing key) would skip the metamethod and
// silently succeed, defeating the whole point. With the visible table
// empty, *every* read misses to __index and *every* write triggers
// __newindex, regardless of whether the key was an "original" variant
// or a new one.
func freezeEnum(_ *vm.VM, args []vm.Value) []vm.Value {
	values := vm.TableArg(freezeGlobalName, 1, args)
	name := vm.StringArg(freezeGlobalName, 2, args)
	return []vm.Value{freeze(name, values)}
}

// freeze wraps a populated `values` table in an immutable proxy: reads route
// through __index to `values`, writes hit __newindex and raise, and
// __metatable is locked so the guard can't be swapped out. Shared by the
// classic enum path and the tagged-enum namespace.
func freeze(name string, values *vm.Table) *vm.Table {
	proxy := vm.NewTable(0, 0)

	mt := vm.NewTable(0, 3)
	mt.Set("__index", values)
	mt.Set("__newindex", &vm.GoFunc{
		Name: "enum.__newindex",
		Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			// Lua-style: raise rather than silently succeed. The enum
			// name is captured here so the message identifies *which*
			// enum the offender tried to mutate — much more useful
			// than a generic "modify enum" when several enums are in
			// scope.
			panic(vm.Errorf("attempt to modify enum '%s'", name))
		},
	})
	// Locking __metatable defeats the most obvious workaround:
	// `setmetatable(Color, {})` to swap out our __newindex. With this
	// set, setmetatable/getmetatable refuse to operate on the proxy.
	mt.Set("__metatable", "enum")

	proxy.SetMetatable(mt)
	return proxy
}

// defineADT implements __enum_adt(name, arities) for tagged enums. It builds
// a namespace table where each variant becomes either a constructor function
// (payload arity > 0) or a singleton tagged value (arity 0), then freezes the
// namespace so variants can't be reassigned. Each produced value carries a
// visible `__tag` field (the variant name — the discriminant `match` reads)
// and a metatable whose `__type` is the enum name (so `typeof` reports it).
func defineADT(_ *vm.VM, args []vm.Value) []vm.Value {
	name := vm.StringArg(adtGlobalName, 1, args)
	arities := vm.TableArg(adtGlobalName, 2, args)

	ns := vm.NewTable(0, 0)
	// Iterate the arities map (variant name -> payload arity).
	var key vm.Value
	for {
		k, v := arities.Next(key)
		if k == nil {
			break
		}
		key = k
		variant, _ := k.(string)
		arity, _ := v.(int64)
		if arity <= 0 {
			ns.Set(variant, makeSingleton(name, variant))
		} else {
			ns.Set(variant, makeVariantConstructor(name, variant, int(arity)))
		}
	}

	return []vm.Value{freeze(name, ns)}
}

// instanceMetatable builds the metatable shared shape for tagged values: a
// __type driving `typeof` and a __tostring rendering `Enum.Variant(...)`.
func instanceMetatable(enum, variant string) *vm.Table {
	mt := vm.NewTable(0, 2)
	mt.Set("__type", enum)
	mt.Set("__tostring", &vm.GoFunc{
		Name: enum + "." + variant + ".__tostring",
		Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
			var self *vm.Table
			if len(a) > 0 {
				self, _ = a[0].(*vm.Table)
			}
			var b strings.Builder
			b.WriteString(enum)
			b.WriteString(".")
			b.WriteString(variant)
			if self != nil && self.Get(int64(1)) != nil {
				b.WriteString("(")
				for i := int64(1); ; i++ {
					val := self.Get(i)
					if val == nil {
						break
					}
					if i > 1 {
						b.WriteString(", ")
					}
					b.WriteString(vm.ToString(val))
				}
				b.WriteString(")")
			}
			return []vm.Value{b.String()}
		},
	})
	return mt
}

// makeSingleton builds the one shared value for a nullary variant. Because
// there is a single instance, `s == Enum.Unit` (reference equality) is a
// valid nullary-variant match.
func makeSingleton(enum, variant string) *vm.Table {
	inst := vm.NewTable(0, 1)
	inst.Set("__tag", variant)
	inst.SetMetatable(instanceMetatable(enum, variant))
	return inst
}

// makeVariantConstructor builds the constructor for a payload variant. It
// binds `arity` positional arguments to integer keys 1..arity; extra args
// are ignored and missing ones are nil.
func makeVariantConstructor(enum, variant string, arity int) *vm.GoFunc {
	return &vm.GoFunc{
		Name: enum + "." + variant,
		Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			inst := vm.NewTable(arity, 1)
			inst.Set("__tag", variant)
			for i := range arity {
				if i < len(args) {
					inst.Set(int64(i+1), args[i])
				}
			}
			inst.SetMetatable(instanceMetatable(enum, variant))
			return []vm.Value{inst}
		},
	}
}
