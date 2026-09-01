package vm

import (
	"math"
	"math/rand"
)

func buildMathLibrary() *Table {
	t := NewTable(0, 32)
	t.Set("pi", math.Pi)
	t.Set("huge", math.Inf(1))
	t.Set("maxinteger", int64(math.MaxInt64))
	t.Set("mininteger", int64(math.MinInt64))

	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "math." + name, Fn: fn})
	}

	add("abs", func(_ *VM, args []Value) []Value {
		i, f, isInt, ok := NumArg("math.abs", 1, args)
		if !ok {
			return []Value{nil}
		}
		if isInt {
			if i < 0 {
				return []Value{-i}
			}
			return []Value{i}
		}
		return []Value{math.Abs(f)}
	})
	add("ceil", roundToInt("math.ceil", math.Ceil))
	add("floor", roundToInt("math.floor", math.Floor))
	add("sqrt", floatToFloat1("math.sqrt", math.Sqrt))
	add("exp", floatToFloat1("math.exp", math.Exp))
	add("sin", floatToFloat1("math.sin", math.Sin))
	add("cos", floatToFloat1("math.cos", math.Cos))
	add("tan", floatToFloat1("math.tan", math.Tan))
	add("asin", floatToFloat1("math.asin", math.Asin))
	add("acos", floatToFloat1("math.acos", math.Acos))
	add("atan", func(_ *VM, args []Value) []Value {
		y := FloatArg("math.atan", 1, args)
		if len(args) >= 2 {
			x := FloatArg("math.atan", 2, args)
			return []Value{math.Atan2(y, x)}
		}
		return []Value{math.Atan(y)}
	})
	add("log", func(_ *VM, args []Value) []Value {
		x := FloatArg("math.log", 1, args)
		if len(args) >= 2 {
			b := FloatArg("math.log", 2, args)
			return []Value{math.Log(x) / math.Log(b)}
		}
		return []Value{math.Log(x)}
	})
	add("max", func(_ *VM, args []Value) []Value {
		best := AnyArg("math.max", 1, args)
		for _, a := range args[1:] {
			if mathLess(best, a) {
				best = a
			}
		}
		return []Value{best}
	})
	add("min", func(_ *VM, args []Value) []Value {
		best := AnyArg("math.min", 1, args)
		for _, a := range args[1:] {
			if mathLess(a, best) {
				best = a
			}
		}
		return []Value{best}
	})
	add("fmod", func(_ *VM, args []Value) []Value {
		if len(args) >= 2 {
			if xi, okx := args[0].(int64); okx {
				if yi, oky := args[1].(int64); oky {
					if yi == 0 {
						panic(LuaError("bad argument #2 to 'math.fmod' (zero)"))
					}
					return []Value{xi % yi}
				}
			}
		}
		x := FloatArg("math.fmod", 1, args)
		y := FloatArg("math.fmod", 2, args)
		return []Value{math.Mod(x, y)}
	})
	add("modf", func(_ *VM, args []Value) []Value {
		x := FloatArg("math.modf", 1, args)
		intPart, fracPart := math.Modf(x)
		return []Value{intPart, fracPart}
	})
	add("pow", func(_ *VM, args []Value) []Value {
		x := FloatArg("math.pow", 1, args)
		y := FloatArg("math.pow", 2, args)
		return []Value{math.Pow(x, y)}
	})
	add("ult", func(_ *VM, args []Value) []Value {
		a := IntArg("math.ult", 1, args)
		b := IntArg("math.ult", 2, args)
		return []Value{uint64(a) < uint64(b)}
	})
	add("tointeger", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return []Value{nil}
		}
		if i, ok := ToInteger(args[0]); ok {
			return []Value{i}
		}
		return []Value{nil}
	})
	add("type", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return []Value{nil}
		}
		switch args[0].(type) {
		case int64:
			return []Value{"integer"}
		case float64:
			return []Value{"float"}
		}
		return []Value{nil}
	})

	rng := rand.New(rand.NewSource(1))
	add("random", func(_ *VM, args []Value) []Value {
		switch len(args) {
		case 0:
			return []Value{rng.Float64()}
		case 1:
			m := IntArg("math.random", 1, args[:1])
			if m == 0 {
				return []Value{int64(rng.Uint64())}
			}
			if m < 0 {
				panic(LuaError("bad argument #1 to 'math.random' (interval is empty)"))
			}
			return []Value{int64(rng.Int63n(m)) + 1}
		default:
			lo := IntArg("math.random", 1, args)
			hi := IntArg("math.random", 2, args)
			if hi < lo {
				panic(LuaError("bad argument #2 to 'math.random' (interval is empty)"))
			}
			span := uint64(hi) - uint64(lo)
			if span == ^uint64(0) {
				return []Value{int64(rng.Uint64())}
			}
			return []Value{lo + int64(rng.Uint64()%(span+1))}
		}
	})
	add("randomseed", func(_ *VM, args []Value) []Value {
		seed := int64(1)
		if len(args) >= 1 && args[0] != nil {
			seed = IntArg("math.randomseed", 1, args)
		}
		rng = rand.New(rand.NewSource(seed))
		return []Value{seed, int64(0)}
	})
	return t
}

func mathLess(a, b Value) bool {
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
	}
	panic(Errorf("bad argument to 'math.max' or 'math.min' (number expected, got %s)", TypeName(b)))
}

func roundToInt(name string, round func(float64) float64) func(*VM, []Value) []Value {
	return func(_ *VM, args []Value) []Value {
		if i, f, isInt, ok := NumArg(name, 1, args); ok {
			if isInt {
				return []Value{i}
			}
			r := round(f)
			if n, ok := floatToInt(r); ok {
				return []Value{n}
			}
			return []Value{r}
		}
		return []Value{nil}
	}
}

func floatToFloat1(name string, fn func(float64) float64) func(*VM, []Value) []Value {
	return func(_ *VM, args []Value) []Value {
		x := FloatArg(name, 1, args)
		return []Value{fn(x)}
	}
}
