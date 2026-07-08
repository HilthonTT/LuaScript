// Package structrt provides the runtime glue the bytecode generator emits
// when lowering a `struct` declaration. It installs ONE global helper,
// `__struct_define(name, fieldNames) -> constructor`, mirroring the way
// native/enumrt installs `__enum_freeze`.
//
// A `struct Point { x: number, y: number }` lowers to
//
//	local Point = __struct_define("Point", {"x", "y"})
//
// The returned constructor is a plain callable. It supports both call
// forms the language exposes:
//
//	Point(1, 2)          -- positional: fields assigned in declaration order
//	Point{ x = 1, y = 2 } -- named: Lua's `f{}` call sugar, one table arg
//
// A single table argument is treated as NAMED construction unless it carries
// an array element at index 1 (in which case it is positional). Instances
// carry a metatable whose `__type` is the struct name — so `typeof(p)`
// reports "Point" — and a `__tostring` for readable printing. Instances are
// ordinary mutable tables otherwise (records, not frozen values).
package structrt

import (
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

const defineGlobalName = "__struct_define"

// RegisterStructRT installs __struct_define on the VM globals. Idempotent,
// like the other runtime registrars, so the REPL can replay it on :reset.
func RegisterStructRT(v *vm.VM) {
	v.SetGlobal(defineGlobalName, &vm.GoFunc{
		Name: defineGlobalName,
		Fn:   defineStruct,
	})
}

// defineStruct implements __struct_define. Args: (name, fieldNames). It
// returns a constructor GoFunc closed over the struct name and ordered
// field-name list.
func defineStruct(_ *vm.VM, args []vm.Value) []vm.Value {
	name := vm.StringArg(defineGlobalName, 1, args)
	fieldsTbl := vm.TableArg(defineGlobalName, 2, args)

	fields := make([]string, 0, fieldsTbl.Len())
	for i := int64(1); i <= fieldsTbl.Len(); i++ {
		if s, ok := fieldsTbl.Get(i).(string); ok {
			fields = append(fields, s)
		}
	}

	ctor := &vm.GoFunc{
		Name: name,
		Fn:   makeConstructor(name, fields),
	}
	return []vm.Value{ctor}
}

// makeConstructor builds the field-populating constructor for one struct.
func makeConstructor(name string, fields []string) func(*vm.VM, []vm.Value) []vm.Value {
	return func(_ *vm.VM, args []vm.Value) []vm.Value {
		inst := vm.NewTable(0, len(fields))

		if named, ok := namedArg(args); ok {
			// Named construction: pull each declared field out of the table,
			// ignoring undeclared keys. Missing fields stay nil.
			for _, f := range fields {
				inst.Set(f, named.Get(f))
			}
		} else {
			// Positional construction: bind args to fields in order. Extra
			// args beyond the field count are ignored; missing ones are nil.
			for i, f := range fields {
				if i < len(args) {
					inst.Set(f, args[i])
				}
			}
		}

		inst.SetMetatable(structMetatable(name, fields))
		return []vm.Value{inst}
	}
}

// namedArg reports whether the call is the single-table named form and, if
// so, returns that table. A lone table argument is treated as named unless
// it has an array element at index 1 (which signals a positional single
// table value the user meant to store, not a field map).
func namedArg(args []vm.Value) (*vm.Table, bool) {
	if len(args) != 1 {
		return nil, false
	}
	t, ok := args[0].(*vm.Table)
	if !ok {
		return nil, false
	}
	if t.Get(int64(1)) != nil {
		return nil, false
	}
	return t, true
}

// structMetatable builds the per-instance metatable. It is rebuilt per
// instance (cheap; three entries) rather than shared, keeping the helper
// stateless — matching enumrt's style. The __type drives `typeof`, and
// __tostring gives records a readable form.
func structMetatable(name string, fields []string) *vm.Table {
	mt := vm.NewTable(0, 2)
	mt.Set("__type", name)
	mt.Set("__tostring", &vm.GoFunc{
		Name: name + ".__tostring",
		Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
			var self *vm.Table
			if len(a) > 0 {
				self, _ = a[0].(*vm.Table)
			}
			if self == nil {
				return []vm.Value{name + "{}"}
			}
			var b strings.Builder
			b.WriteString(name)
			b.WriteString("{ ")
			for i, f := range fields {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(f)
				b.WriteString(" = ")
				b.WriteString(vm.ToString(self.Get(f)))
			}
			b.WriteString(" }")
			return []vm.Value{b.String()}
		},
	})
	return mt
}
