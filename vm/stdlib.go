package vm

import (
	"fmt"
	"strings"
)

// registerStdlib installs a minimal set of Lua built-in globals: print, type,
// tostring, tonumber, ipairs, pairs, next, error, pcall, assert, select.
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
	g("assert", builtinAssert)
	g("select", builtinSelect)

	// Metatable controls.
	g("setmetatable", builtinSetmetatable)
	g("getmetatable", builtinGetmetatable)
	g("rawget", builtinRawget)
	g("rawset", builtinRawset)
	g("rawequal", builtinRawequal)
	g("rawlen", builtinRawlen)
}

// ---------------------------------------------------------------------------
// Metatable / raw access globals
// ---------------------------------------------------------------------------

func builtinSetmetatable(_ *VM, args []Value) []Value {
	t := TableArg("setmetatable", 1, args)
	// Lua 5.4: if the existing metatable has a `__metatable` field, the
	// metatable is "locked" and setmetatable raises.
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
	// Lua: if __metatable is set, return that instead — protects from
	// tampering. Honor the convention.
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
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = ToStringMM(v, a)
	}
	fmt.Println(strings.Join(parts, "\t"))
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
	i, f, isInt, ok := ToNumber(args[0])
	if !ok {
		return []Value{nil}
	}
	if isInt {
		return []Value{i}
	}
	return []Value{f}
}

// ipairs iterator: (state, ctrl) -> next-int-key, value or nil to stop.
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

// pairs iterator wraps Table.Next.
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
	switch m := msg.(type) {
	case string:
		panic(LuaError(m))
	case nil:
		panic(LuaError("nil"))
	default:
		panic(LuaError(ToStringMM(v, m)))
	}
}

func builtinPcall(v *VM, args []Value) []Value {
	fn := AnyArg("pcall", 1, args)
	callArgs := args[1:]
	results, err := safeCall(v, fn, callArgs)
	if err != nil {
		return []Value{false, err.Error()}
	}
	return append([]Value{true}, results...)
}

func safeCall(v *VM, fn Value, args []Value) (rs []Value, err error) {
	// Snapshot frame depth so we can unwind any frames the failing call
	// pushed before bubbling the error back to the pcall caller.
	frameDepth := len(v.frames)
	stackTop := len(v.Stack)

	defer func() {
		if r := recover(); r != nil {
			// Discard frames/stack pushed during the failing call.
			v.frames = v.frames[:frameDepth]
			v.Stack = v.Stack[:stackTop]
			switch e := r.(type) {
			case LuaError:
				err = e
			case error:
				err = e
			default:
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	rs = v.CallValue(fn, args, -1)
	return
}

func builtinAssert(_ *VM, args []Value) []Value {
	if len(args) == 0 || !IsTruthy(args[0]) {
		msg := "assertion failed!"
		if len(args) >= 2 {
			msg = ToString(args[1])
		}
		panic(LuaError(msg))
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
