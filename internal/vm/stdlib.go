package vm

import (
	"os"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/internal/gctune"
)

type luaError struct{ value Value }

func (e luaError) Error() string {
	return ToString(e.value)
}

func registerStdlib(v *VM) {
	g := func(name string, fn func(*VM, []Value) []Value) {
		v.Globals.Set(name, &GoFunc{Name: name, Fn: fn})
	}

	g("print", builtinPrint)
	g("type", builtinType)
	g("tostring", builtinTostring)
	g("tonumber", builtinTonumber)
	g("ipairs", builtinIpairs)
	g("pairs", builtinPairs)
	g("next", builtinNext)
	g("error", builtinError)
	g("pcall", builtinPcall)
	g("xpcall", builtinXpcall)
	g("assert", builtinAssert)
	g("select", builtinSelect)
	g("collectgarbage", builtinCollectgarbage)
	g("typeof", builtinTypeof)
	g("sizeof", builtinSizeof)

	v.Globals.Set("_G", v.Globals)
	v.Globals.Set("_VERSION", "Lua 5.4")

	g("setmetatable", builtinSetmetatable)
	g("getmetatable", builtinGetmetatable)
	g("rawget", builtinRawget)
	g("rawset", builtinRawset)
	g("rawequal", builtinRawequal)
	g("rawlen", builtinRawlen)
}

func builtinSetmetatable(_ *VM, args []Value) []Value {
	t := TableArg("setmetatable", 1, args)
	if existing := t.Metatable(); existing != nil {
		if existing.Get("__metatable") != nil {
			panic(LuaError("cannot change a protected metatable"))
		}
	}
	t.SetMetatable(NilOrTableArg("setmetatable", 2, args))
	return []Value{t}
}

func builtinGetmetatable(_ *VM, args []Value) []Value {
	if len(args) < 1 {
		return []Value{nil}
	}
	t, ok := args[0].(*Table)
	if !ok {
		return []Value{nil}
	}
	mt := t.Metatable()
	if mt == nil {
		return []Value{nil}
	}
	if mm := mt.Get("__metatable"); mm != nil {
		return []Value{mm}
	}
	return []Value{mt}
}

func builtinRawget(_ *VM, args []Value) []Value {
	t := TableArg("rawget", 1, args)
	key := AnyArg("rawget", 2, args)
	return []Value{t.Get(key)}
}

func builtinRawset(_ *VM, args []Value) []Value {
	t := TableArg("rawset", 1, args)
	key := AnyArg("rawset", 2, args)
	val := AnyArg("rawset", 3, args)
	t.Set(key, val)
	return []Value{t}
}

func builtinRawequal(_ *VM, args []Value) []Value {
	if len(args) < 2 {
		return []Value{false}
	}
	return []Value{Equal(args[0], args[1])}
}

func builtinRawlen(_ *VM, args []Value) []Value {
	s, t, isString := TableOrStringArg("rawlen", 1, args)
	if isString {
		return []Value{int64(len(s))}
	}
	return []Value{t.Len()}
}

func builtinPrint(v *VM, args []Value) []Value {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte('\t')
		}
		b.WriteString(ToStringMM(v, a))
	}
	b.WriteByte('\n')
	os.Stdout.WriteString(b.String())
	return nil
}

func builtinType(_ *VM, args []Value) []Value {
	return []Value{TypeName(AnyArg("type", 1, args))}
}

func builtinTostring(v *VM, args []Value) []Value {
	if len(args) == 0 {
		return []Value{"nil"}
	}
	return []Value{ToStringMM(v, args[0])}
}

func builtinTonumber(_ *VM, args []Value) []Value {
	if len(args) == 0 {
		return []Value{nil}
	}
	if len(args) >= 2 && args[1] != nil {
		base, ok := ToInteger(args[1])
		if !ok || base < 2 || base > 36 {
			panic(Errorf("bad argument #2 to 'tonumber' (base out of range)"))
		}
		s, ok := args[0].(string)
		if !ok {
			panic(Errorf("bad argument #1 to 'tonumber' (string expected, got %s)", TypeName(args[0])))
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), int(base), 64)
		if err != nil {
			return []Value{nil}
		}
		return []Value{n}
	}
	i, f, isInt, ok := ToNumber(args[0])
	if !ok {
		return []Value{nil}
	}
	if isInt {
		return []Value{i}
	}
	return []Value{f}
}

func builtinTypeof(v *VM, args []Value) []Value {
	if len(args) == 0 {
		return []Value{"nil"}
	}

	switch x := args[0].(type) {
	case nil:
		return []Value{"nil"}
	case bool:
		return []Value{"boolean"}
	case int64:
		return []Value{"integer"}
	case float64:
		return []Value{"float"}
	case string:
		return []Value{"string"}
	case *Table:
		if mt := x.Metatable(); mt != nil {
			if name, ok := mt.Get("__type").(string); ok {
				return []Value{name}
			}
		}
		return []Value{"table"}
	case *Closure, *GoFunc:
		return []Value{"function"}
	case *Coroutine:
		return []Value{"thread"}
	default:
		return []Value{TypeName(x)}
	}
}

func builtinSizeof(_ *VM, args []Value) []Value {
	if len(args) == 0 {
		return []Value{int64(0)}
	}

	switch x := args[0].(type) {
	case nil:
		return []Value{int64(0)}
	case bool:
		return []Value{int64(1)}
	case int64:
		return []Value{int64(8)}
	case float64:
		return []Value{int64(8)}
	case string:
		return []Value{int64(len(x))}
	case *Table:
		return []Value{x.EntryCount()}
	default:
		return []Value{int64(8)}
	}
}

func ipairsIter(_ *VM, args []Value) []Value {
	t, ok := args[0].(*Table)
	if !ok {
		return []Value{nil}
	}
	i, _ := ToInteger(args[1])
	i++
	val := t.Get(i)
	if val == nil {
		return []Value{nil}
	}
	return []Value{i, val}
}

func builtinIpairs(_ *VM, args []Value) []Value {
	t := TableArg("ipairs", 1, args)
	return []Value{
		&GoFunc{Name: "ipairs:iter", Fn: ipairsIter},
		t,
		int64(0),
	}
}

func pairsIter(_ *VM, args []Value) []Value {
	t, ok := args[0].(*Table)
	if !ok {
		return []Value{nil}
	}
	k, val := t.Next(args[1])
	if k == nil {
		return []Value{nil}
	}
	return []Value{k, val}
}

func builtinPairs(_ *VM, args []Value) []Value {
	t := TableArg("pairs", 1, args)
	return []Value{
		&GoFunc{Name: "pairs:iter", Fn: pairsIter},
		t,
		nil,
	}
}

func builtinNext(_ *VM, args []Value) []Value {
	t := TableArg("next", 1, args)
	var key Value
	if len(args) > 1 {
		key = args[1]
	}
	k, val := t.Next(key)
	if k == nil {
		return []Value{nil}
	}
	return []Value{k, val}
}

func builtinError(v *VM, args []Value) []Value {
	var msg Value
	if len(args) > 0 {
		msg = args[0]
	}
	level := int64(1)
	if len(args) > 1 {
		if l, ok := ToInteger(args[1]); ok {
			level = l
		}
	}
	if s, ok := msg.(string); ok && level > 0 {
		msg = v.where(int(level)) + s
	}
	panic(luaError{value: msg})
}

func builtinPcall(v *VM, args []Value) []Value {
	fn := AnyArg("pcall", 1, args)
	callArgs := args[1:]
	results, errVal, _, failed := safeCall(v, fn, callArgs, false)
	if failed {
		return []Value{false, errVal}
	}
	return append([]Value{true}, results...)
}

func builtinXpcall(v *VM, args []Value) []Value {
	fn := AnyArg("xpcall", 1, args)
	handler := AnyArg("xpcall", 2, args)
	results, errVal, _, failed := safeCall(v, fn, args[2:], false)
	if !failed {
		return append([]Value{true}, results...)
	}
	hres, herr, _, hfailed := safeCall(v, handler, []Value{errVal}, false)
	if hfailed {
		return []Value{false, herr}
	}
	var hv Value
	if len(hres) > 0 {
		hv = hres[0]
	}
	return []Value{false, hv}
}

func (v *VM) SafeCall(fn Value, args []Value) (rs []Value, errVal Value, failed bool) {
	rs, errVal, _, failed = safeCall(v, fn, args, false)
	return
}

func (v *VM) SafeCallTrace(fn Value, args []Value) (rs []Value, errVal Value, stack []TracebackEntry, failed bool) {
	return safeCall(v, fn, args, true)
}

func safeCall(v *VM, fn Value, args []Value, wantTrace bool) (rs []Value, errVal Value, stack []TracebackEntry, failed bool) {
	frameDepth := len(v.frames)
	stackTop := len(v.Stack)
	markDepth := len(v.callMarks)

	defer func() {
		if r := recover(); r != nil {
			if isCloseSignal(r) {
				panic(r)
			}
			errVal = v.errorValue(r)
			if wantTrace {
				stack = v.tracebackFrom(frameDepth)
			}
			for i := len(v.frames) - 1; i >= frameDepth; i-- {
				if len(v.frames[i].Deferred) > 0 {
					v.runDeferredSafely(v.frames[i])
				}
			}
			v.closeUpvaluesAbove(stackTop)
			v.frames = v.frames[:frameDepth]
			v.Stack = v.Stack[:stackTop]
			v.callMarks = v.callMarks[:markDepth]
			failed = true
		}
	}()
	rs = v.CallValue(fn, args, -1)
	return
}

func builtinAssert(v *VM, args []Value) []Value {
	if len(args) == 0 || !IsTruthy(args[0]) {
		if len(args) >= 2 {
			panic(luaError{value: args[1]})
		}
		panic(luaError{value: v.where(1) + "assertion failed!"})
	}
	return args
}

func builtinSelect(_ *VM, args []Value) []Value {
	if len(args) == 0 {
		panic(LuaError("bad argument #1 to 'select' (number expected)"))
	}
	if s, ok := args[0].(string); ok && s == "#" {
		return []Value{int64(len(args) - 1)}
	}
	idx, ok := ToInteger(args[0])
	if !ok {
		panic(Errorf("bad argument #1 to 'select' (number expected, got %s)", TypeName(args[0])))
	}
	rest := args[1:]
	if idx < 0 {
		idx = int64(len(rest)) + idx + 1
	}
	if idx < 1 {
		panic(LuaError("bad argument #1 to 'select' (index out of range)"))
	}
	if int(idx) > len(rest) {
		return nil
	}
	return append([]Value(nil), rest[idx-1:]...)
}

func builtinCollectgarbage(_ *VM, args []Value) []Value {
	opt := OptString("collectgarbage", 1, args, "collect")
	switch opt {
	case "collect":
		gctune.Collect()
		return []Value{int64(0)}
	case "step":
		gctune.Collect()
		return []Value{true}
	case "stop":
		gctune.Stop()
		return []Value{int64(0)}
	case "restart":
		gctune.Restart()
		return []Value{int64(0)}
	case "count":
		b := gctune.HeapBytes()
		return []Value{float64(b) / 1024, int64(b % 1024)}
	case "setpause":
		prev := gctune.SetPercent(int(OptInt("collectgarbage", 2, args, gctune.DefaultPercent)))
		return []Value{int64(prev)}
	case "setstepmul":
		return []Value{int64(0)}
	case "isrunning":
		return []Value{gctune.IsRunning()}
	case "incremental", "generational":
		return []Value{"incremental"}
	default:
		panic(Errorf("bad argument #1 to 'collectgarbage' (invalid option '%s')", opt))
	}
}
