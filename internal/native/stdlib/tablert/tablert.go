package tablert

import "github.com/hilthontt/luascript/internal/vm"

const (
	spreadGlobalName     = "__tbl_spread"
	pushGlobalName       = "__tbl_push"
	restArrayGlobalName  = "__rest_array"
	restRecordGlobalName = "__rest_record"
)

func RegisterTableRT(v *vm.VM) {
	v.SetGlobal(spreadGlobalName, &vm.GoFunc{Name: spreadGlobalName, Fn: spreadInto})
	v.SetGlobal(pushGlobalName, &vm.GoFunc{Name: pushGlobalName, Fn: push})
	v.SetGlobal(restArrayGlobalName, &vm.GoFunc{Name: restArrayGlobalName, Fn: restArray})
	v.SetGlobal(restRecordGlobalName, &vm.GoFunc{Name: restRecordGlobalName, Fn: restRecord})
}

// spreadInto implements `__tbl_spread(dest, cursor, src)`: src's array part
// goes to dest[cursor], dest[cursor+1], ..., its other keys are copied as-is,
// and the next free cursor is returned. The cursor is threaded through the
// generated code instead of recomputed from dest.Len(), because a nil element
// would make the length stop short and later elements overwrite earlier ones.
func spreadInto(_ *vm.VM, args []vm.Value) []vm.Value {
	dest := vm.TableArg(spreadGlobalName, 1, args)
	cursor := vm.IntArg(spreadGlobalName, 2, args)
	src := vm.TableArg(spreadGlobalName, 3, args)

	n := src.Len()
	for i := int64(1); i <= n; i++ {
		dest.Set(cursor, src.Get(i))
		cursor++
	}
	for k, val := src.Next(nil); k != nil; k, val = src.Next(k) {
		if idx, isInt := k.(int64); isInt && idx >= 1 && idx <= n {
			continue
		}
		dest.Set(k, val)
	}
	return []vm.Value{cursor}
}

// push implements `__tbl_push(dest, cursor, ...)`: each value is stored at the
// next position, nils included, and the next free cursor is returned.
func push(_ *vm.VM, args []vm.Value) []vm.Value {
	dest := vm.TableArg(pushGlobalName, 1, args)
	cursor := vm.IntArg(pushGlobalName, 2, args)
	if len(args) < 3 {
		// A trailing call that returned no values contributes nothing.
		return []vm.Value{cursor}
	}
	for _, val := range args[2:] {
		dest.Set(cursor, val)
		cursor++
	}
	return []vm.Value{cursor}
}

func restArray(_ *vm.VM, args []vm.Value) []vm.Value {
	src, ok := args[0].(*vm.Table)
	if !ok {
		return []vm.Value{vm.NewTable(0, 0)}
	}
	from := int64(1)
	if len(args) > 1 {
		if n, isInt := args[1].(int64); isInt {
			from = n
		}
	}
	n := src.Len()
	out := vm.NewTable(int(max(0, n-from+1)), 0)
	for i := from; i <= n; i++ {
		out.Set(i-from+1, src.Get(i))
	}
	return []vm.Value{out}
}

func restRecord(_ *vm.VM, args []vm.Value) []vm.Value {
	src, ok := args[0].(*vm.Table)
	if !ok {
		return []vm.Value{vm.NewTable(0, 0)}
	}
	skip := map[string]bool{}
	if len(args) > 1 {
		if keys, isTable := args[1].(*vm.Table); isTable {
			for i := int64(1); i <= keys.Len(); i++ {
				if s, isStr := keys.Get(i).(string); isStr {
					skip[s] = true
				}
			}
		}
	}
	out := vm.NewTable(0, 0)
	for k, val := src.Next(nil); k != nil; k, val = src.Next(k) {
		if s, isStr := k.(string); isStr && skip[s] {
			continue
		}
		out.Set(k, val)
	}
	return []vm.Value{out}
}
