package debugx

import (
	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterDebugPreload(v *vm.VM) {
	vm.RegisterPreload(v, "debug", debugLoader)
}

func debugLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newDebug()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newDebug() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 4)

	methods.Set("traceback", &vm.GoFunc{Name: "debug:traceback", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		msg := ""
		if len(args) >= 1 {
			if s, ok := args[0].(string); ok {
				msg = s
			} else if args[0] != nil {
				return []vm.Value{args[0]}
			}
		}
		level := vm.OptInt("debug.traceback", 2, args, 1)
		return []vm.Value{formatTraceback(v, msg, int(level))}
	}})

	methods.Set("getinfo", &vm.GoFunc{Name: "debug:getinfo", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 1 {
			panic(vm.Errorf("bad argument #1 to 'debug.getinfo' (number or function expected)"))
		}
		switch x := args[0].(type) {
		case int64:
			info := frameInfo(v, int(x))
			if info == nil {
				return []vm.Value{nil}
			}
			return []vm.Value{info}
		case float64:
			info := frameInfo(v, int(x))
			if info == nil {
				return []vm.Value{nil}
			}
			return []vm.Value{info}
		case *vm.Closure:
			return []vm.Value{closureInfo(x)}
		case *vm.GoFunc:
			return []vm.Value{goFuncInfo(x)}
		}
		panic(vm.Errorf("bad argument #1 to 'debug.getinfo' (number or function expected, got %s)", vm.TypeName(args[0])))
	}})

	methods.Set("sethook", &vm.GoFunc{Name: "debug:sethook", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return nil
	}})
	methods.Set("gethook", &vm.GoFunc{Name: "debug:gethook", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{nil, "", int64(0)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

func formatTraceback(v *vm.VM, msg string, level int) string {
	skip := level - 1
	if skip < 0 {
		skip = 0
	}
	tb := vm.FormatTraceback(v.Traceback(skip))
	switch {
	case msg == "":
		return tb
	case tb == "":
		return msg
	}
	return msg + "\n" + tb
}

func frameInfo(v *vm.VM, level int) *vm.Table {
	frames := v.CallFrames()
	idx := len(frames) - level
	if level <= 0 || idx < 0 || idx >= len(frames) {
		return nil
	}
	t := closureInfo(frames[idx].Closure)
	if entries := v.Traceback(level - 1); len(entries) > 0 {
		t.Set("currentline", int64(entries[0].Line))
	}
	return t
}

func closureInfo(c *vm.Closure) *vm.Table {
	t := vm.NewTable(0, 8)
	t.Set("what", "Lua")
	t.Set("source", protoSource(c))
	t.Set("short_src", protoSource(c))
	t.Set("currentline", int64(-1))
	t.Set("name", protoName(c))
	t.Set("namewhat", "")
	t.Set("nparams", int64(c.Proto.NumParams))
	t.Set("isvararg", c.Proto.IsVararg)
	return t
}

func goFuncInfo(g *vm.GoFunc) *vm.Table {
	t := vm.NewTable(0, 4)
	t.Set("what", "C")
	t.Set("source", "=[C]")
	t.Set("short_src", "[C]")
	t.Set("currentline", int64(-1))
	t.Set("name", g.Name)
	t.Set("namewhat", "")
	return t
}

func protoSource(c *vm.Closure) string {
	if c == nil || c.Proto == nil {
		return "[?]"
	}
	return c.Proto.Source()
}

func protoName(c *vm.Closure) string {
	if c == nil || c.Proto == nil {
		return ""
	}
	return c.Proto.Name()
}
