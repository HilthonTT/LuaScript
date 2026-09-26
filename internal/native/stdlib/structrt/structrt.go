package structrt

import (
	"slices"
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

const defineGlobalName = "__struct_define"

func RegisterStructRT(v *vm.VM) {
	v.SetGlobal(defineGlobalName, &vm.GoFunc{
		Name: defineGlobalName,
		Fn:   defineStruct,
	})
}

func defineStruct(_ *vm.VM, args []vm.Value) []vm.Value {
	name := vm.StringArg(defineGlobalName, 1, args)
	fieldsTbl := vm.TableArg(defineGlobalName, 2, args)

	fields := make([]string, 0, fieldsTbl.Len())
	for i := int64(1); i <= fieldsTbl.Len(); i++ {
		if s, ok := fieldsTbl.Get(i).(string); ok {
			fields = append(fields, s)
		}
	}

	class := vm.NewTable(0, 4)
	instanceMeta := instanceMetatable(name, fields, class)

	classMeta := vm.NewTable(0, 3)
	classMeta.Set("__type", name)
	classMeta.Set("__call", &vm.GoFunc{
		Name: name,
		Fn:   makeConstructor(name, fields, instanceMeta),
	})
	classMeta.Set("__tostring", &vm.GoFunc{
		Name: name + ".__tostring",
		Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			return []vm.Value{name}
		},
	})
	class.SetMetatable(classMeta)

	return []vm.Value{class}
}

func makeConstructor(_ string, fields []string, meta *vm.Table) func(*vm.VM, []vm.Value) []vm.Value {
	return func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) > 0 {
			args = args[1:]
		}

		inst := vm.NewTable(0, len(fields))
		if named, ok := namedArg(args, fields); ok {
			for _, f := range fields {
				inst.Set(f, named.Get(f))
			}
		} else {
			for i, f := range fields {
				if i < len(args) {
					inst.Set(f, args[i])
				}
			}
		}
		inst.SetMetatable(meta)
		return []vm.Value{inst}
	}
}

func namedArg(args []vm.Value, fields []string) (*vm.Table, bool) {
	if len(args) != 1 {
		return nil, false
	}
	t, ok := args[0].(*vm.Table)
	if !ok || t.Metatable() != nil {
		return nil, false
	}
	for k, _ := t.Next(nil); k != nil; k, _ = t.Next(k) {
		s, isStr := k.(string)
		if !isStr || !slices.Contains(fields, s) {
			return nil, false
		}
	}
	return t, true
}

func instanceMetatable(name string, fields []string, class *vm.Table) *vm.Table {
	mt := vm.NewTable(0, 3)
	mt.Set("__type", name)
	mt.Set("__index", class)
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
