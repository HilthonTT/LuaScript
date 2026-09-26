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

func spreadInto(_ *vm.VM, args []vm.Value) []vm.Value {
	dest := vm.TableArg(spreadGlobalName, 1, args)
	pos := vm.IntArg(spreadGlobalName, 2, args)
	src := vm.TableArg(spreadGlobalName, 3, args)

	n := src.Len()
	for i := int64(1); i <= n; i++ {
		dest.Set(pos+i, src.Get(i))
	}
	for k, val := src.Next(nil); k != nil; k, val = src.Next(k) {
		if idx, isInt := k.(int64); isInt && idx >= 1 && idx <= n {
			continue
		}
		dest.Set(k, val)
	}
	return []vm.Value{pos + n}
}

func push(_ *vm.VM, args []vm.Value) []vm.Value {
	dest := vm.TableArg(pushGlobalName, 1, args)
	pos := vm.IntArg(pushGlobalName, 2, args)
	for i, val := range args[min(2, len(args)):] {
		if val != nil {
			dest.Set(pos+int64(i)+1, val)
		}
	}
	return []vm.Value{pos + int64(max(0, len(args)-2))}
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
