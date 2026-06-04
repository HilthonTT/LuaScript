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

import "github.com/hilthontt/luascript/vm"

// freezeGlobalName is the global slot the bytecode generator looks up
// when lowering an enum declaration. Kept as a named constant so a
// future rename touches both sites in one place.
const freezeGlobalName = "__enum_freeze"

// RegisterEnumRT installs __enum_freeze on the VM's globals. Idempotent —
// calling it twice replaces the binding with the same function, matching
// how the other registrars behave when the REPL replays them on :reset.
func RegisterEnumRT(v *vm.VM) {
	v.SetGlobal(freezeGlobalName, &vm.GoFunc{
		Name: freezeGlobalName,
		Fn:   freezeEnum,
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
	return []vm.Value{proxy}
}
