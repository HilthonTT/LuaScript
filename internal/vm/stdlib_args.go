package vm

import "reflect"

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

func StringArg(name string, n int, args []Value) string {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (string expected)", n, name))
	}
	if s, ok := args[n-1].(string); ok {
		return s
	}
	if i, f, isInt, ok := ToNumber(args[n-1]); ok {
		if isInt {
			return formatInteger(i)
		}
		return formatFloat(f)
	}
	panic(Errorf("bad argument #%d to '%s' (string expected, got %s)", n, name, describeBadArg(args[n-1])))
}

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

func AnyArg(name string, n int, args []Value) Value {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (value expected)", n, name))
	}
	return args[n-1]
}

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

func OptString(name string, n int, args []Value, dflt string) string {
	if n < 1 || n > len(args) {
		return dflt
	}
	if args[n-1] == nil {
		return dflt
	}
	return StringArg(name, n, args)
}

func OptInt(name string, n int, args []Value, dflt int64) int64 {
	if n < 1 || n > len(args) {
		return dflt
	}
	if args[n-1] == nil {
		return dflt
	}
	return IntArg(name, n, args)
}
