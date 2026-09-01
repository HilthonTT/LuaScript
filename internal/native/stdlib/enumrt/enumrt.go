package enumrt

import (
	"strings"

	"github.com/hilthontt/luascript/internal/vm"
)

const freezeGlobalName = "__enum_freeze"

const adtGlobalName = "__enum_adt"

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

func freezeEnum(_ *vm.VM, args []vm.Value) []vm.Value {
	values := vm.TableArg(freezeGlobalName, 1, args)
	name := vm.StringArg(freezeGlobalName, 2, args)
	return []vm.Value{freeze(name, values)}
}

func freeze(name string, values *vm.Table) *vm.Table {
	proxy := vm.NewTable(0, 0)

	mt := vm.NewTable(0, 3)
	mt.Set("__index", values)
	mt.Set("__newindex", &vm.GoFunc{
		Name: "enum.__newindex",
		Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			panic(vm.Errorf("attempt to modify enum '%s'", name))
		},
	})
	mt.Set("__metatable", "enum")

	proxy.SetMetatable(mt)
	return proxy
}

func defineADT(_ *vm.VM, args []vm.Value) []vm.Value {
	name := vm.StringArg(adtGlobalName, 1, args)
	arities := vm.TableArg(adtGlobalName, 2, args)

	ns := vm.NewTable(0, 0)
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

func makeSingleton(enum, variant string) *vm.Table {
	inst := vm.NewTable(0, 1)
	inst.Set("__tag", variant)
	inst.SetMetatable(instanceMetatable(enum, variant))
	return inst
}

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
