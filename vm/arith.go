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
			return float64(x) < y
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return x < float64(y)
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
			return float64(x) <= y
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return x <= float64(y)
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func firstNonNumber(a, b Value) Value {
	if _, _, _, ok := ToNumber(a); !ok {
		return a
	}
	return b
}
