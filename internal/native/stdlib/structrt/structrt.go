package structrt

import (
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

	ctor := &vm.GoFunc{
		Name: name,
		Fn:   makeConstructor(name, fields),
	}
	return []vm.Value{ctor}
}

func makeConstructor(name string, fields []string) func(*vm.VM, []vm.Value) []vm.Value {
	return func(_ *vm.VM, args []vm.Value) []vm.Value {
		inst := vm.NewTable(0, len(fields))

		if named, ok := namedArg(args); ok {
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

		inst.SetMetatable(structMetatable(name, fields))
		return []vm.Value{inst}
	}
}

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
