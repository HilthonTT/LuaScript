package vm

import (
	"fmt"
	"strings"
)

type Value = any

func IsTruthy(v Value) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

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
