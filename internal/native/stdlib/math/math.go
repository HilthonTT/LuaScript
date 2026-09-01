package math

import (
	"math"
	"math/rand"

	"github.com/hilthontt/luascript/internal/native"
	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterMathPreload(v *vm.VM) {
	vm.RegisterPreload(v, "math", mathLoader)
}

func mathLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newMath()
	mod.Set("VERSION", "0.1.0")

	mod.Set("e", math.E)
	mod.Set("pi", math.Pi)
	mod.Set("phi", math.Phi)

	mod.Set("sqrt2", math.Sqrt2)
	mod.Set("sqrte", math.SqrtE)
	mod.Set("sqrtpi", math.SqrtPi)
	mod.Set("sqrtphi", math.SqrtPhi)

	mod.Set("ln2", math.Ln2)
	mod.Set("log2e", math.Log2E)
	mod.Set("ln10", math.Ln10)
	mod.Set("log10e", math.Log10E)

	mod.Set("maxfloat32", math.MaxFloat32)
	mod.Set("smallestnonzerofloat32", math.SmallestNonzeroFloat32)
	mod.Set("maxfloat64", math.MaxFloat64)
	mod.Set("smallestnonzerofloat64", math.SmallestNonzeroFloat64)

	mod.Set("maxint", math.MaxInt)
	mod.Set("minint", math.MinInt)
	mod.Set("maxint8", math.MaxInt8)
	mod.Set("minint8", math.MinInt8)
	mod.Set("maxint16", math.MaxInt16)
	mod.Set("minint16", math.MinInt16)
	mod.Set("maxint32", math.MaxInt32)
	mod.Set("minint32", math.MinInt32)
	mod.Set("maxint64", math.MaxInt64)
	mod.Set("minint64", math.MinInt64)
	mod.Set("maxuint8", math.MaxUint8)
	mod.Set("maxuint16", math.MaxUint16)
	mod.Set("maxuint32", math.MaxUint32)

	mod.Set("huge", math.Inf(1))
	mod.Set("nan", math.NaN())

	return []vm.Value{mod}
}

func unary(name string, fn func(float64) float64) *vm.GoFunc {
	return &vm.GoFunc{Name: "math:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg(name, 1, args)
		return []vm.Value{fn(x)}
	}}
}

func newMath() *vm.Table {
	m := vm.NewTable(0, 8)
	methods := vm.NewTable(0, 32)

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
	methods.Set("cbrt", unary("cbrt", math.Cbrt))
	methods.Set("erf", unary("erf", math.Erf))
	methods.Set("erfc", unary("erfc", math.Erfc))
	methods.Set("deg", unary("deg", deg))
	methods.Set("rad", unary("rad", rad))

	methods.Set("floor", &vm.GoFunc{Name: "math:floor", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("floor", 1, args)
		return []vm.Value{toIntIfPossible(math.Floor(x))}
	}})

	methods.Set("ceil", &vm.GoFunc{Name: "math:ceil", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("ceil", 1, args)
		return []vm.Value{toIntIfPossible(math.Ceil(x))}
	}})

	methods.Set("fmod", &vm.GoFunc{Name: "math:fmod", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) >= 2 {
			if xi, okx := args[0].(int64); okx {
				if yi, oky := args[1].(int64); oky {
					if yi == 0 {
						panic(vm.LuaError("bad argument #2 to 'fmod' (zero)"))
					}
					return []vm.Value{xi % yi}
				}
			}
		}
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		return []vm.Value{math.Mod(x, y)}
	}})

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

	methods.Set("tointeger", &vm.GoFunc{Name: "math:tointeger", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		switch vm.AnyArg("tointeger", 1, args).(type) {
		case int64, float64:
			if i, ok := vm.ToInteger(args[0]); ok {
				return []vm.Value{i}
			}
		}
		return []vm.Value{nil}
	}})

	methods.Set("ult", &vm.GoFunc{Name: "math:ult", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.IntArg("m", 1, args)
		y := vm.IntArg("n", 2, args)
		return []vm.Value{uint64(x) < uint64(y)}
	}})

	methods.Set("random", &vm.GoFunc{Name: "math:random", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		switch len(args) {
		case 0:
			return []vm.Value{rand.Float64()}
		case 1:
			upper := vm.IntArg("m", 1, args)
			if upper == 0 {
				return []vm.Value{int64(rand.Uint64())}
			}
			if upper < 0 {
				panic(vm.Errorf("bad argument #1 to 'random' (interval is empty)"))
			}
			return []vm.Value{1 + rand.Int63n(upper)}
		default:
			lower := vm.IntArg("m", 1, args)
			upper := vm.IntArg("n", 2, args)
			if lower > upper {
				panic(vm.Errorf("bad argument #2 to 'random' (interval is empty)"))
			}
			span := uint64(upper) - uint64(lower)
			if span == ^uint64(0) {
				return []vm.Value{int64(rand.Uint64())}
			}
			return []vm.Value{lower + int64(rand.Uint64()%(span+1))}
		}
	}})

	methods.Set("pow", &vm.GoFunc{Name: "math:pow", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		return []vm.Value{math.Pow(x, y)}
	}})

	methods.Set("max", &vm.GoFunc{Name: "math:max", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{native.MaxOf(numbersArg("math.max", args))}
	}})

	methods.Set("min", &vm.GoFunc{Name: "math:min", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{native.MinOf(numbersArg("math.min", args))}
	}})

	methods.Set("mean", &vm.GoFunc{Name: "math:mean", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{native.Meanf(numbersArg("math.mean", args))}
	}})

	methods.Set("variance", &vm.GoFunc{Name: "math:variance", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{native.Variancef(numericTableArg("math.variance", args))}
	}})

	methods.Set("standard_deviation", &vm.GoFunc{Name: "math:standard_deviation", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{native.Stddevf(numericTableArg("math.standard_deviation", args))}
	}})

	methods.Set("softmax", &vm.GoFunc{Name: "math:softmax", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{floatSliceToTable(native.Softmax(numericTableArg("math.softmax", args)))}
	}})

	methods.Set("clamp", &vm.GoFunc{Name: "math:clamp", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		min := vm.FloatArg("min", 2, args)
		max := vm.FloatArg("max", 3, args)
		return []vm.Value{native.Clamp(x, min, max)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

func toIntIfPossible(f float64) vm.Value {
	if i, ok := vm.ToInteger(f); ok {
		return i
	}
	return f
}

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

func numericTableArg(site string, args []vm.Value) []float64 {
	t := vm.TableArg(site, 1, args)
	return floatSliceFromTable(site, t)
}

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

func floatSliceToTable(xs []float64) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	native.WriteBack(t, xs)
	return t
}
