package math

import (
	"math"
	"math/rand"

	"github.com/hilthontt/luascript/native"
	"github.com/hilthontt/luascript/vm"
)

func RegisterMathPreload(v *vm.VM) {
	vm.RegisterPreload(v, "math", mathLoader)
}

func mathLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newMath()
	mod.Set("VERSION", "0.1.0")
	mod.Set("huge", math.Inf(1))
	mod.Set("maxinteger", int64(math.MaxInt64))
	mod.Set("mininteger", int64(math.MinInt64))
	mod.Set("pi", float64(math.Pi))
	mod.Set("e", float64(math.E))
	mod.Set("nan", math.NaN())
	mod.Set("phi", float64(math.Phi))

	return []vm.Value{mod}
}

// unary wraps a float64 -> float64 math function as a GoFunc, so the dozens of
// trig/log entries below don't each repeat the same arg-extraction boilerplate.
func unary(name string, fn func(float64) float64) *vm.GoFunc {
	return &vm.GoFunc{Name: "math:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg(name, 1, args)
		return []vm.Value{fn(x)}
	}}
}

func newMath() *vm.Table {
	m := vm.NewTable(0, 8)
	methods := vm.NewTable(0, 32)

	// abs preserves the integer/float subtype, matching Lua 5.4: math.abs(-3)
	// is the integer 3, math.abs(-3.0) is the float 3.0. Negating mininteger
	// overflows back to itself, which is also Lua's documented behaviour.
	methods.Set("abs", &vm.GoFunc{Name: "math:abs", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if i, ok := vm.AnyArg("abs", 1, args).(int64); ok {
			if i < 0 {
				return []vm.Value{-i}
			}
			return []vm.Value{i}
		}
		x := vm.FloatArg("abs", 1, args)
		return []vm.Value{math.Abs(x)}
	}})

	methods.Set("cos", unary("cos", math.Cos))
	methods.Set("cosh", unary("cosh", math.Cosh))
	methods.Set("sin", unary("sin", math.Sin))
	methods.Set("sinh", unary("sinh", math.Sinh))
	methods.Set("tan", unary("tan", math.Tan))
	methods.Set("tanh", unary("tanh", math.Tanh))
	methods.Set("acos", unary("acos", math.Acos))
	methods.Set("acosh", unary("acosh", math.Acosh))
	methods.Set("asin", unary("asin", math.Asin))
	methods.Set("asinh", unary("asinh", math.Asinh))
	methods.Set("atan", unary("atan", math.Atan))
	methods.Set("atanh", unary("atanh", math.Atanh))
	methods.Set("exp", unary("exp", math.Exp))
	methods.Set("sqrt", unary("sqrt", math.Sqrt))
	methods.Set("deg", unary("deg", deg))
	methods.Set("rad", unary("rad", rad))

	// floor and ceil return an integer when the result fits in int64 (Lua 5.4
	// semantics), so values flow straight into table indices and print as "3"
	// rather than "3.0". Out-of-range results fall back to a float.
	methods.Set("floor", &vm.GoFunc{Name: "math:floor", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("floor", 1, args)
		return []vm.Value{toIntIfPossible(math.Floor(x))}
	}})

	methods.Set("ceil", &vm.GoFunc{Name: "math:ceil", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("ceil", 1, args)
		return []vm.Value{toIntIfPossible(math.Ceil(x))}
	}})

	methods.Set("fmod", &vm.GoFunc{Name: "math:fmod", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		return []vm.Value{math.Mod(x, y)}
	}})

	// log(x [, base]). Lua defaults to natural log; base 2 and 10 use the
	// dedicated, more-accurate library functions, anything else is computed as
	// log(x)/log(base).
	methods.Set("log", &vm.GoFunc{Name: "math:log", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		if len(args) >= 2 && args[1] != nil {
			base := vm.FloatArg("base", 2, args)
			switch base {
			case 2:
				return []vm.Value{math.Log2(x)}
			case 10:
				return []vm.Value{math.Log10(x)}
			default:
				return []vm.Value{math.Log(x) / math.Log(base)}
			}
		}
		return []vm.Value{math.Log(x)}
	}})

	methods.Set("modf", &vm.GoFunc{Name: "math:modf", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		intPart, fracPart := math.Modf(value)
		return []vm.Value{toIntIfPossible(intPart), fracPart}
	}})

	// tointeger returns the integer value of a number, or nil if it has no
	// exact integer representation. Unlike the rest of the module it does not
	// coerce strings — that mirrors Lua, where math.tointeger("3") is nil.
	methods.Set("tointeger", &vm.GoFunc{Name: "math:tointeger", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		switch vm.AnyArg("tointeger", 1, args).(type) {
		case int64, float64:
			if i, ok := vm.ToInteger(args[0]); ok {
				return []vm.Value{i}
			}
		}
		return []vm.Value{nil}
	}})

	// ult compares two integers as unsigned 64-bit values (Lua's math.ult).
	methods.Set("ult", &vm.GoFunc{Name: "math:ult", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.IntArg("m", 1, args)
		y := vm.IntArg("n", 2, args)
		return []vm.Value{uint64(x) < uint64(y)}
	}})

	// random follows Lua's three forms:
	//   random()      -> float in [0, 1)
	//   random(m)     -> integer in [1, m]
	//   random(m, n)  -> integer in [m, n]
	methods.Set("random", &vm.GoFunc{Name: "math:random", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		switch len(args) {
		case 0:
			return []vm.Value{rand.Float64()}
		case 1:
			upper := vm.IntArg("m", 1, args)
			if upper < 1 {
				panic(vm.Errorf("bad argument #1 to 'random' (interval is empty)"))
			}
			return []vm.Value{1 + rand.Int63n(upper)}
		default:
			lower := vm.IntArg("m", 1, args)
			upper := vm.IntArg("n", 2, args)
			if lower > upper {
				panic(vm.Errorf("bad argument #2 to 'random' (interval is empty)"))
			}
			return []vm.Value{lower + rand.Int63n(upper-lower+1)}
		}
	}})

	methods.Set("pow", &vm.GoFunc{Name: "math:pow", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		return []vm.Value{math.Pow(x, y)}
	}})

	// max / min accept either a single array table or a variadic list of
	// numbers: math.max({1, 5, 3}) and math.max(1, 5, 3) both return 5.
	methods.Set("max", &vm.GoFunc{Name: "math:max", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{maxOf(numbersArg("math.max", args))}
	}})

	methods.Set("min", &vm.GoFunc{Name: "math:min", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{minOf(numbersArg("math.min", args))}
	}})

	methods.Set("mean", &vm.GoFunc{Name: "math:mean", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{mean(numbersArg("math.mean", args))}
	}})

	methods.Set("variance", &vm.GoFunc{Name: "math:variance", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{variance(numericTableArg("math.variance", args))}
	}})

	methods.Set("standard_deviation", &vm.GoFunc{Name: "math:standard_deviation", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{standardDeviation(numericTableArg("math.standard_deviation", args))}
	}})

	methods.Set("softmax", &vm.GoFunc{Name: "math:softmax", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{floatSliceToTable(softmax(numericTableArg("math.softmax", args)))}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// toIntIfPossible narrows an integer-valued float to int64 when it fits,
// otherwise returns the float unchanged.
func toIntIfPossible(f float64) vm.Value {
	if i, ok := vm.ToInteger(f); ok {
		return i
	}
	return f
}

// numbersArg gathers the numbers a reducer (max/min/mean) should operate on.
// A lone table argument is treated as an array; otherwise every argument must
// be a number. An empty result is an error — there is no max of nothing.
func numbersArg(site string, args []vm.Value) []float64 {
	var nums []float64
	if len(args) == 1 {
		if t, ok := args[0].(*vm.Table); ok {
			nums = floatSliceFromTable(site, t)
		}
	}
	if nums == nil {
		nums = make([]float64, len(args))
		for i := range args {
			nums[i] = vm.FloatArg(site, i+1, args)
		}
	}
	if len(nums) == 0 {
		panic(vm.Errorf("bad argument #1 to '%s' (number or non-empty array expected)", site))
	}
	return nums
}

// numericTableArg extracts the array part of the first (table) argument as a
// []float64. Used by the stats functions, which only make sense over an array.
func numericTableArg(site string, args []vm.Value) []float64 {
	t := vm.TableArg(site, 1, args)
	return floatSliceFromTable(site, t)
}

// floatSliceFromTable reads t's array part into a []float64, promoting ints.
// A string (or otherwise non-numeric) array is rejected rather than silently
// treated as zeros.
func floatSliceFromTable(site string, t *vm.Table) []float64 {
	arr, err := native.ExtractArray(t)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}

	switch a := arr.(type) {
	case []int64:
		out := make([]float64, len(a))
		for i, v := range a {
			out[i] = float64(v)
		}
		return out
	case []float64:
		return a
	default:
		panic(vm.Errorf("%s: expected an array of numbers", site))
	}
}

// floatSliceToTable boxes a Go slice back into a Lua array table.
func floatSliceToTable(xs []float64) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	native.WriteBack(t, xs)
	return t
}
