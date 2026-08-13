package vm

import (
	"os"
	"strconv"
	"strings"

	"github.com/hilthontt/luascript/internal/gctune"
)

// luaError carries an arbitrary Lua error value raised by error(v) through a Go
// panic unwind, so pcall/xpcall and coroutine.resume can hand back the original
// value (table, number, …) rather than a stringified version of it. VM-internal
// runtime errors still panic with the string-typed LuaError.
type luaError struct{ value Value }

func (e luaError) Error() string {
	return ToString(e.value)
}

// (VM.errorValue in traceback.go maps a recovered panic to the Lua error value
// a protected call surfaces, stamping the raise position onto VM-raised ones.)

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
	g("xpcall", builtinXpcall)
	g("assert", builtinAssert)
	g("select", builtinSelect)
	g("collectgarbage", builtinCollectgarbage)
	g("typeof", builtinTypeof)
	g("sizeof", builtinSizeof)

	// _G is the globals table itself, so `_G["x"] = 1` and `_G[name]` give
	// scripts dynamic access to globals whose names aren't known statically.
	// Self-referential by design in Lua (_G._G == _G).
	v.Globals.Set("_G", v.Globals)
	// _VERSION names the language level scripts can branch on. This runtime
	// tracks Lua 5.4 semantics, so it reports the string 5.4 code tests for
	// rather than a LuaScript-specific string no existing script checks.
	v.Globals.Set("_VERSION", "Lua 5.4")

	// Metatable controls.
	g("setmetatable", builtinSetmetatable)
	g("getmetatable", builtinGetmetatable)
	g("rawget", builtinRawget)
	g("rawset", builtinRawset)
	g("rawequal", builtinRawequal)
	g("rawlen", builtinRawlen)
}

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
	// tonumber(s, base): interpret s as an integer literal in [2,36].
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

// builtinTypeof is a finer-grained companion to `type`. Where `type` collapses
// both integer and float subtypes into "number", typeof reports them
// separately ("integer"/"float"), and for tables it honours a `__type` string
// field on the metatable (Luau-style) so user-defined classes can name
// themselves. Everything else matches `type`'s tags.
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

// builtinSizeof reports the in-memory size of a value in bytes, with C-like
// semantics for the fixed-width scalar types:
//
//	nil       → 0
//	boolean   → 1
//	integer   → 8   (int64)
//	float     → 8   (float64)
//	string    → byte length of the contents
//	table     → number of key/value entries it holds
//
// Functions and threads are reference types; sizeof reports the size of the
// reference (8 bytes on a 64-bit host).
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
	level := int64(1)
	if len(args) > 1 {
		if l, ok := ToInteger(args[1]); ok {
			level = l
		}
	}
	// Lua prefixes a *string* message with the raising position unless
	// level is 0: level 1 is where error was called, level 2 its caller, …
	// Non-string values propagate unchanged (tables, numbers, …);
	// pcall/xpcall return them verbatim.
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
	// The message handler runs protected as well; an error inside it
	// replaces the original one rather than escaping the xpcall.
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

// SafeCall invokes fn with args, recovering any Lua error or runtime panic and
// fully unwinding the VM (frames, stack, and any open upvalues the failing call
// created) so the VM stays usable afterwards. It returns the call results, the
// Lua error value (valid only when failed is true), and whether the call
// failed. Hosts that run user callbacks outside a pcall — e.g. the http
// server's per-request handler dispatch — should route through this so one bad
// callback can't corrupt the shared VM.
func (v *VM) SafeCall(fn Value, args []Value) (rs []Value, errVal Value, failed bool) {
	rs, errVal, _, failed = safeCall(v, fn, args, false)
	return
}

// SafeCallTrace is SafeCall plus the Lua call stack as it stood at the raise
// point, innermost frame first — the same capture an uncaught error carries in
// RuntimeError.Stack. Only the frames the call itself pushed are included, so a
// host reporting a failed callback shows the callback's stack rather than the
// chunk that registered it. `stack` is nil unless failed is true.
//
// Kept separate from SafeCall because capturing allocates, and pcall — which is
// on hot paths — has no use for it.
func (v *VM) SafeCallTrace(fn Value, args []Value) (rs []Value, errVal Value, stack []TracebackEntry, failed bool) {
	return safeCall(v, fn, args, true)
}

func safeCall(v *VM, fn Value, args []Value, wantTrace bool) (rs []Value, errVal Value, stack []TracebackEntry, failed bool) {
	// Snapshot frame/stack depth so we can unwind anything the failing call
	// pushed before bubbling the error back to the pcall caller.
	frameDepth := len(v.frames)
	stackTop := len(v.Stack)
	markDepth := len(v.callMarks)

	defer func() {
		if r := recover(); r != nil {
			// coroutine.close's unwind sentinel must not be catchable by
			// pcall; let it keep unwinding to the coroutine's goroutine body.
			if isCloseSignal(r) {
				panic(r)
			}
			// Resolve the error value FIRST, while v.frames still holds the
			// failing call stack: that is what lets a VM-raised error be
			// stamped with the position it was raised at. Everything below
			// destroys those frames.
			errVal = v.errorValue(r)
			// Same rule as errorValue: capture while the failing frames are
			// still standing. frameDepth trims the caller's stack, leaving
			// only what this call pushed.
			if wantTrace {
				stack = v.tracebackFrom(frameDepth)
			}
			// Run deferred calls for every frame abandoned by this unwind,
			// innermost first, so `defer` cleanup still happens when an error
			// propagates and not only on a normal return. runDeferredSafely
			// contains a panic from any single cleanup so it can't replace the
			// original error. Done before the truncation below, while the
			// failing frames' locals and open upvalues are still live.
			for i := len(v.frames) - 1; i >= frameDepth; i-- {
				if len(v.frames[i].Deferred) > 0 {
					v.runDeferredSafely(v.frames[i])
				}
			}
			// Close any upvalues the failing call left open above the
			// snapshot before truncating the stack out from under them —
			// otherwise they dangle with an index into freed slots and
			// crash the VM on a later read.
			v.closeUpvaluesAbove(stackTop)
			v.frames = v.frames[:frameDepth]
			v.Stack = v.Stack[:stackTop]
			// An error thrown between a MarkArgs and its matching Call
			// leaves pending marks behind; drop them or the enclosing
			// variadic call pops a stale mark and reads a bogus args base.
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
			// A supplied message is the error object and propagates
			// unchanged, position included or not, exactly as thrown.
			panic(luaError{value: args[1]})
		}
		// The default message is raised by assert itself, so it carries the
		// position of the failing call the way any other builtin's error does.
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

// builtinCollectgarbage implements Lua 5.4's collectgarbage(opt [, arg]) on top
// of the host's gctune knobs. Because the VM's values are ordinary Go heap
// objects, "collection" is really Go's GC — the options map onto runtime/debug
// rather than a bespoke collector:
//
//	collect / step     -> runtime.GC()        (returns 0 / true)
//	stop               -> disable GC          (returns 0)
//	restart            -> re-enable GC         (returns 0)
//	count              -> live heap in KBytes  (returns Kbytes, bytes%1024)
//	setpause [, n]     -> set GOGC             (returns previous percent)
//	setstepmul         -> no Go equivalent     (accepted; returns 0)
//	isrunning          -> GC enabled?          (returns bool)
//	incremental/gen…   -> mode is not selectable on Go's GC (returns "incremental")
func builtinCollectgarbage(_ *VM, args []Value) []Value {
	opt := OptString("collectgarbage", 1, args, "collect")
	switch opt {
	case "collect":
		gctune.Collect()
		return []Value{int64(0)}
	case "step":
		// Go's GC has no incremental "step"; run a full cycle and report a
		// completed collection, matching Lua's true-means-finished contract.
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
		// Accepted for source compatibility; Go's GC exposes no step multiplier.
		return []Value{int64(0)}
	case "isrunning":
		return []Value{gctune.IsRunning()}
	case "incremental", "generational":
		// Go's collector mode isn't selectable; report the prior mode like Lua.
		return []Value{"incremental"}
	default:
		panic(Errorf("bad argument #1 to 'collectgarbage' (invalid option '%s')", opt))
	}
}
