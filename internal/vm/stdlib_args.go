package vm

// Argument-validation helpers for stdlib and native-module callbacks.
//
// Every Lua callable implemented in Go (builtins, library functions, host
// modules) receives a `[]Value` and must validate it before unpacking. The
// raw shape of those checks is mechanical: presence test, type check, and
// an error message in Lua's "bad argument #N to 'funcname' (TYPE expected,
// got X)" format. The helpers in this file centralize that shape so each
// call site reads as a single line and so the FFI-cast hint that
// `describeBadArg` adds (for raw Go ints / floats / named strings) fires
// uniformly across every stdlib and native callback.
//
// Error-message conventions follow Lua 5.4:
//   - Missing arg:  "bad argument #N to 'name' (TYPE expected)"
//   - Wrong type:   "bad argument #N to 'name' (TYPE expected, got <desc>)"
// where <desc> is `describeBadArg(args[n-1])`. Optional-argument helpers
// (Opt*) silently substitute a default when the arg is absent, but still
// raise on a wrong-type present value.

import "reflect"

// describeBadArg renders the type of a bad argument for error messages.
// For runtime-tracked values it falls through to TypeName, matching Lua's
// `type()` strings. For values that crossed an FFI boundary as a raw Go
// primitive, it appends an actionable hint — the most common host-module
// bug is forgetting to cast int / uint / FileMode / rune to int64 (or
// float32 to float64) before storing them on a *Table, which leaves the
// runtime with an opaque value it can't coerce.
func describeBadArg(v Value) string {
	base := TypeName(v)
	switch v.(type) {
	case nil, bool, int64, float64, string, *Table, *Closure, *GoFunc, *Coroutine:
		return base
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Uintptr:
		return base + " — host stored a Go integer; cast to int64 before passing it to the runtime"
	case reflect.Float32:
		return base + " — host stored a Go float32; cast to float64 before passing it to the runtime"
	case reflect.String:
		return base + " — host stored a non-string named string type; convert to plain `string` before passing it to the runtime"
	}
	return base
}

// Numeric helpers

// NumArg accepts any value coercible to a Lua number (int or float). The
// (i, f, isInt, ok) tuple lets callers preserve integer-vs-float subtype.
// `ok` is always true on return — a non-coercible arg panics directly.
func NumArg(name string, n int, args []Value) (int64, float64, bool, bool) {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (value expected)", n, name))
	}
	i, f, isInt, ok := ToNumber(args[n-1])
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (number expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return i, f, isInt, ok
}

// FloatArg coerces to float64 (integer → float promotion is fine).
func FloatArg(name string, n int, args []Value) float64 {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (number expected)", n, name))
	}
	x, ok := ToFloat(args[n-1])
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (number expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return x
}

// IntArg coerces to int64. Floats with a fractional part are rejected.
func IntArg(name string, n int, args []Value) int64 {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (number expected)", n, name))
	}
	x, ok := ToInteger(args[n-1])
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (number expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return x
}

// StringArg accepts strings, and (per Lua) coerces numbers via the standard
// integer/float formatter.
func StringArg(name string, n int, args []Value) string {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (string expected)", n, name))
	}
	if s, ok := args[n-1].(string); ok {
		return s
	}
	if i, f, isInt, ok := ToNumber(args[n-1]); ok {
		// Lua allows numbers where strings are expected via implicit coercion.
		if isInt {
			return formatInteger(i)
		}
		return formatFloat(f)
	}
	panic(Errorf("bad argument #%d to '%s' (string expected, got %s)", n, name, describeBadArg(args[n-1])))
}

// Object helpers

// TableArg insists on a *Table.
func TableArg(name string, n int, args []Value) *Table {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (table expected)", n, name))
	}
	t, ok := args[n-1].(*Table)
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (table expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return t
}

// ClosureArg insists on a Lua *Closure. (Use FunctionArg if you also want
// to accept host *GoFunc; this helper is strict, matching Lua's
// `coroutine.create` / `coroutine.wrap` which require a real closure.)
func ClosureArg(name string, n int, args []Value) *Closure {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (function expected)", n, name))
	}
	cl, ok := args[n-1].(*Closure)
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (function expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return cl
}

// CoroutineArg insists on a *Coroutine.
func CoroutineArg(name string, n int, args []Value) *Coroutine {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (coroutine expected)", n, name))
	}
	co, ok := args[n-1].(*Coroutine)
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (coroutine expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return co
}

// AnyArg simply asserts presence — for "value expected" sites where the
// callee handles every Value type itself (type(), pcall(), error()).
func AnyArg(name string, n int, args []Value) Value {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (value expected)", n, name))
	}
	return args[n-1]
}

// NilOrTableArg accepts nil (returned as Go nil *Table) or a *Table. Used
// by setmetatable's second argument.
func NilOrTableArg(name string, n int, args []Value) *Table {
	if n < 1 || n > len(args) {
		return nil
	}
	switch v := args[n-1].(type) {
	case nil:
		return nil
	case *Table:
		return v
	}
	panic(Errorf("bad argument #%d to '%s' (nil or table expected, got %s)", n, name, describeBadArg(args[n-1])))
}

// TableOrStringArg accepts either a string or a *Table — used by rawlen.
// Exactly one of the returned `s`/`t` will be meaningful; the other is the
// zero value. Callers should branch on `isString`.
func TableOrStringArg(name string, n int, args []Value) (s string, t *Table, isString bool) {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (table or string expected)", n, name))
	}
	switch v := args[n-1].(type) {
	case string:
		return v, nil, true
	case *Table:
		return "", v, false
	}
	panic(Errorf("bad argument #%d to '%s' (table or string expected, got %s)", n, name, describeBadArg(args[n-1])))
}

// Optional-argument helpers

// OptString returns the string arg at position n or `dflt` if the arg is
// absent or nil. A present, non-string, non-nil value raises.
func OptString(name string, n int, args []Value, dflt string) string {
	if n < 1 || n > len(args) {
		return dflt
	}
	if args[n-1] == nil {
		return dflt
	}
	return StringArg(name, n, args)
}

// OptInt returns the integer arg at position n or `dflt` if the arg is
// absent or nil. A present, non-numeric, non-nil value raises.
func OptInt(name string, n int, args []Value, dflt int64) int64 {
	if n < 1 || n > len(args) {
		return dflt
	}
	if args[n-1] == nil {
		return dflt
	}
	return IntArg(name, n, args)
}
