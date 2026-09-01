package vm

import (
	"fmt"
	"math"
)

type LuaError string

func (e LuaError) Error() string {
	return string(e)
}

func Errorf(format string, args ...any) LuaError {
	return LuaError(fmt.Sprintf(format, args...))
}

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

func intFitsFloat(i int64) bool {
	const lim = int64(1) << 53
	return i >= -lim && i <= lim
}

func floatFloorToInt(f float64) (int64, bool) { return floatToIntBound(math.Floor(f)) }
func floatCeilToInt(f float64) (int64, bool)  { return floatToIntBound(math.Ceil(f)) }

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
	if fi, ok := floatCeilToInt(f); ok {
		return i < fi
	}
	return f > 0
}

func leIntFloat(i int64, f float64) bool {
	if intFitsFloat(i) {
		return float64(i) <= f
	}
	if fi, ok := floatFloorToInt(f); ok {
		return i <= fi
	}
	return f > 0
}

func ltFloatInt(f float64, i int64) bool {
	if intFitsFloat(i) {
		return f < float64(i)
	}
	if fi, ok := floatFloorToInt(f); ok {
		return fi < i
	}
	return f < 0
}

func leFloatInt(f float64, i int64) bool {
	if intFitsFloat(i) {
		return f <= float64(i)
	}
	if fi, ok := floatCeilToInt(f); ok {
		return fi <= i
	}
	return f < 0
}

func firstNonNumber(a, b Value) Value {
	if _, _, _, ok := ToNumber(a); !ok {
		return a
	}
	return b
}
