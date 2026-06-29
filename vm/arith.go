package vm

import (
	"fmt"
	"math"
)

// LuaError is a typed string used when the VM raises a runtime error. It is
// recovered by pcall().
type LuaError string

func (e LuaError) Error() string {
	return string(e)
}

func Errorf(format string, args ...any) LuaError {
	return LuaError(fmt.Sprintf(format, args...))
}

// arith dispatches +, -, *, % keeping the integer subtype when both operands
// are integers. Strings convertible to numbers are accepted.
func arith(a, b Value, op string) Value {
	ai, af, aIsInt, aOk := ToNumber(a)
	bi, bf, bIsInt, bOk := ToNumber(b)
	if !aOk || !bOk {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(firstNonNumber(a, b))))
	}
	if aIsInt && bIsInt {
		return intArith(ai, bi, op)
	}
	x := af
	if aIsInt {
		x = float64(ai)
	}
	y := bf
	if bIsInt {
		y = float64(bi)
	}
	return floatArith(x, y, op)
}

// (intArith lives in integer.go; floatArith lives in float.go.)

// arithDiv is `/` — always returns a float.
func arithDiv(a, b Value) Value {
	x, ok := ToFloat(a)
	if !ok {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(a)))
	}
	y, ok := ToFloat(b)
	if !ok {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(b)))
	}
	return x / y
}

// arithFloorDiv is `//`. int//int=int, otherwise float floor division.
func arithFloorDiv(a, b Value) Value {
	ai, af, aIsInt, aOk := ToNumber(a)
	bi, bf, bIsInt, bOk := ToNumber(b)
	if !aOk || !bOk {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(firstNonNumber(a, b))))
	}
	if aIsInt && bIsInt {
		if bi == 0 {
			panic(LuaError("attempt to perform 'n//0'"))
		}
		q := ai / bi
		// Floor toward -infinity for mismatched signs.
		if (ai%bi != 0) && ((ai < 0) != (bi < 0)) {
			q--
		}
		return q
	}
	x := af
	if aIsInt {
		x = float64(ai)
	}
	y := bf
	if bIsInt {
		y = float64(bi)
	}
	return math.Floor(x / y)
}

// arithPow is `^` — always float.
func arithPow(a, b Value) Value {
	x, ok := ToFloat(a)
	if !ok {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(a)))
	}
	y, ok := ToFloat(b)
	if !ok {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(b)))
	}
	return math.Pow(x, y)
}

// arithNeg unary minus. Preserves integer subtype.
func arithNeg(v Value) Value {
	i, f, isInt, ok := ToNumber(v)
	if !ok {
		panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(v)))
	}
	if isInt {
		return -i
	}
	return -f
}

// ---------------------------------------------------------------------------
// Bitwise — operands must be exactly representable as int64.
// ---------------------------------------------------------------------------

func toBitInt(v Value) int64 {
	i, ok := ToInteger(v)
	if !ok {
		panic(Errorf("bitwise operand has no integer representation (%s)", TypeName(v)))
	}
	return i
}

func bitAnd(a, b Value) Value { return toBitInt(a) & toBitInt(b) }
func bitOr(a, b Value) Value  { return toBitInt(a) | toBitInt(b) }
func bitXor(a, b Value) Value { return toBitInt(a) ^ toBitInt(b) }
func bitNot(a Value) Value    { return ^toBitInt(a) }

// shl / shr use Lua semantics: shift counts beyond the word size return 0,
// negative shift counts shift the other way.
func shl(a, b Value) Value {
	x := toBitInt(a)
	n := toBitInt(b)
	return shiftLeft(x, n)
}

func shr(a, b Value) Value {
	x := toBitInt(a)
	n := toBitInt(b)
	return shiftLeft(x, -n)
}

func shiftLeft(x, n int64) int64 {
	if n >= 64 || n <= -64 {
		return 0
	}
	if n >= 0 {
		return int64(uint64(x) << uint64(n))
	}
	return int64(uint64(x) >> uint64(-n))
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

// less implements Lua's `<`. Numbers cross-compare; strings compare
// lexicographically; mismatched types raise a runtime error.
func less(a, b Value) bool {
	switch x := a.(type) {
	case int64:
		switch y := b.(type) {
		case int64:
			return x < y
		case float64:
			return ltIntFloat(x, y)
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return ltFloatInt(x, y)
		case float64:
			return x < y
		}
	case string:
		if y, ok := b.(string); ok {
			return x < y
		}
	}
	panic(Errorf("attempt to compare %s with %s", TypeName(a), TypeName(b)))
}

// lessOrEqual implements Lua's `<=` mirroring less.
func lessOrEqual(a, b Value) bool {
	switch x := a.(type) {
	case int64:
		switch y := b.(type) {
		case int64:
			return x <= y
		case float64:
			return leIntFloat(x, y)
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return leFloatInt(x, y)
		case float64:
			return x <= y
		}
	case string:
		if y, ok := b.(string); ok {
			return x <= y
		}
	}
	panic(Errorf("attempt to compare %s with %s", TypeName(a), TypeName(b)))
}

// Mixed int/float ordering. Converting the int to float64 loses precision
// above 2^53, so for integers that don't fit exactly in a float we compare
// against the float's floor/ceil as integers — mirroring Lua 5.4's
// LTintfloat / LEintfloat / LTfloatint / LEfloatint.

// intFitsFloat reports whether i is representable exactly as a float64.
func intFitsFloat(i int64) bool {
	const lim = int64(1) << 53 // 2^53: floats are exact for |i| <= 2^53
	return i >= -lim && i <= lim
}

func floatFloorToInt(f float64) (int64, bool) { return floatToIntBound(math.Floor(f)) }
func floatCeilToInt(f float64) (int64, bool)  { return floatToIntBound(math.Ceil(f)) }

// floatToIntBound converts an already-rounded float to int64, failing for NaN
// or out-of-range values (the bound is the nearest representable 2^63).
func floatToIntBound(f float64) (int64, bool) {
	if math.IsNaN(f) || f < -9.2233720368547758e+18 || f >= 9.2233720368547758e+18 {
		return 0, false
	}
	return int64(f), true
}

func ltIntFloat(i int64, f float64) bool {
	if intFitsFloat(i) {
		return float64(i) < f
	}
	if fi, ok := floatCeilToInt(f); ok { // i < f  <=>  i < ceil(f)
		return i < fi
	}
	return f > 0 // f beyond all integers (or NaN -> false)
}

func leIntFloat(i int64, f float64) bool {
	if intFitsFloat(i) {
		return float64(i) <= f
	}
	if fi, ok := floatFloorToInt(f); ok { // i <= f  <=>  i <= floor(f)
		return i <= fi
	}
	return f > 0
}

func ltFloatInt(f float64, i int64) bool {
	if intFitsFloat(i) {
		return f < float64(i)
	}
	if fi, ok := floatFloorToInt(f); ok { // f < i  <=>  floor(f) < i
		return fi < i
	}
	return f < 0
}

func leFloatInt(f float64, i int64) bool {
	if intFitsFloat(i) {
		return f <= float64(i)
	}
	if fi, ok := floatCeilToInt(f); ok { // f <= i  <=>  ceil(f) <= i
		return fi <= i
	}
	return f < 0
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func firstNonNumber(a, b Value) Value {
	if _, _, _, ok := ToNumber(a); !ok {
		return a
	}
	return b
}
