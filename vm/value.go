// Package vm executes the bytecode produced by compiler/bytecode. The model
// is a conventional Lua 5.4 stack VM: every value lives on a single operand
// stack, locals are slots inside the current frame, upvalues capture
// enclosing-function locals, and globals live in a single _G table.
//
// Lua values map to Go types as follows:
//
//	nil       → Lua nil
//	bool      → Lua boolean        (see boolean.go)
//	int64     → Lua integer        (see integer.go)
//	float64   → Lua float          (see float.go)
//	string    → Lua string         (see string.go)
//	*Table    → Lua table          (see table.go)
//	*Closure  → Lua-defined fn     (see closure.go)
//	*GoFunc   → Go-defined fn      (see closure.go)
package vm

import (
	"fmt"
	"strings"
)

// Value is any Lua runtime value. The valid concrete types are documented at
// the top of this file.
type Value = any

// IsTruthy reports whether v is treated as true by Lua control-flow rules.
// Only nil and false are falsy; every other value (including 0 and "") is truthy.
func IsTruthy(v Value) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

// TypeName returns the Lua-level type tag of v ("nil", "boolean", "number",
// "string", "table", "function").
func TypeName(v Value) string {
	switch v.(type) {
	case nil:
		return "nil"
	case bool:
		return "boolean"
	case int64, float64:
		return "number"
	case string:
		return "string"
	case *Table:
		return "table"
	case *Closure, *GoFunc:
		return "function"
	case *Coroutine:
		return "thread"
	default:
		return fmt.Sprintf("userdata(%T)", v)
	}
}

// ToString renders v in Lua's print/tostring format. Per-type formatters live
// in their respective per-type files.
func ToString(v Value) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		return formatBool(x)
	case int64:
		return formatInteger(x)
	case float64:
		return formatFloat(x)
	case string:
		return x
	case *Table:
		return fmt.Sprintf("table: %p", x)
	case *Closure:
		return fmt.Sprintf("function: %p", x)
	case *GoFunc:
		return "function: builtin " + x.Name
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ToStringMM renders val to its string form, honouring the `__tostring`
// metamethod on tables and other values that have one. Numeric/string/bool/
// nil values skip the lookup and route to ToString directly — they cannot
// carry metatables in current.lsc. If `__tostring` is defined but does
// not return a string, Lua's rule applies: it is an error. Here we surface
// that as a panic with a Lua-style message so it lands as a runtime error.
//
// Callers must pass a non-nil *VM. A nil VM falls back to ToString (used by
// disassembly / debug paths that shouldn't trigger Lua calls).
func ToStringMM(v *VM, val Value) string {
	if v == nil {
		return ToString(val)
	}
	switch val.(type) {
	case nil, bool, int64, float64, string:
		return ToString(val)
	}
	if mm := v.getMetamethod(val, "__tostring"); mm != nil {
		res := v.callMM(mm, val)
		if s, ok := res.(string); ok {
			return s
		}
		panic(Errorf("'__tostring' must return a string"))
	}
	return ToString(val)
}

// ToNumber attempts to coerce v to a Lua number. It reports the value's
// concrete Lua subtype: an int64 is an integer, a float64 is a float — even
// when the float is mathematically integral (e.g. 2.0 stays a float, so
// `2.0 + 3` is the float 5.0, per Lua 5.4's int/float subtype rules).
// Returns the value, an "is integer" flag, and an "ok" flag (false if v
// cannot be a number). Strings follow tonumber's rules: "2" parses to an
// integer, "2.0" to a float.
func ToNumber(v Value) (intVal int64, floatVal float64, isInt bool, ok bool) {
	switch x := v.(type) {
	case int64:
		return x, 0, true, true
	case float64:
		return 0, x, false, true
	case string:
		return parseNumber(strings.TrimSpace(x))
	}
	return 0, 0, false, false
}

// ToFloat coerces v to a float64 by Lua rules.
func ToFloat(v Value) (float64, bool) {
	i, f, isInt, ok := ToNumber(v)
	if !ok {
		return 0, false
	}
	if isInt {
		return float64(i), true
	}
	return f, true
}

// ToInteger coerces v to an int64 by Lua rules. Floats only convert if they
// are mathematically integer-valued and fit in int64.
func ToInteger(v Value) (int64, bool) {
	i, f, isInt, ok := ToNumber(v)
	if !ok {
		return 0, false
	}
	if isInt {
		return i, true
	}
	return floatToInt(f)
}

// Equal implements Lua-level == for two values. Numbers cross-compare across
// int/float subtype; everything else compares within type. Tables/functions
// compare by reference. Metatable __eq is layered on top of this in vm.go.
func Equal(a, b Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case int64:
		switch y := b.(type) {
		case int64:
			return x == y
		case float64:
			// Exact: int == float iff the float is an integer equal to x.
			// Converting x to float64 would lose precision above 2^53.
			yi, ok := floatToInt(y)
			return ok && x == yi
		}
		return false
	case float64:
		switch y := b.(type) {
		case int64:
			xi, ok := floatToInt(x)
			return ok && xi == y
		case float64:
			return x == y
		}
		return false
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	return a == b
}
