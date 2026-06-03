package math

import (
	"math"
	"math/rand"

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

	return []vm.Value{mod}
}

func newMath() *vm.Table {
	m := vm.NewTable(0, 6)
	methods := vm.NewTable(0, 25)

	methods.Set("abs", &vm.GoFunc{Name: "math:abs", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		absValue := math.Abs(value)
		return []vm.Value{absValue}
	}})

	methods.Set("cos", &vm.GoFunc{Name: "math:cos", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		cosValue := math.Cos(value)
		return []vm.Value{cosValue}
	}})

	methods.Set("sin", &vm.GoFunc{Name: "math:sin", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		sinValue := math.Sin(value)
		return []vm.Value{sinValue}
	}})

	methods.Set("tan", &vm.GoFunc{Name: "math:tan", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		tanValue := math.Tan(value)
		return []vm.Value{tanValue}
	}})

	methods.Set("acos", &vm.GoFunc{Name: "math:acos", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		acosValue := math.Acos(value)
		return []vm.Value{acosValue}
	}})

	methods.Set("asin", &vm.GoFunc{Name: "math:asin", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		asinValue := math.Asin(value)
		return []vm.Value{asinValue}
	}})

	methods.Set("atan", &vm.GoFunc{Name: "math:atan", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		atanvalue := math.Atan(value)
		return []vm.Value{atanvalue}
	}})

	methods.Set("ceil", &vm.GoFunc{Name: "math:ceil", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		ceiledValue := math.Ceil(value)
		return []vm.Value{ceiledValue}
	}})

	methods.Set("deg", &vm.GoFunc{Name: "math:deg", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		degreeValue := deg(value)
		return []vm.Value{degreeValue}
	}})

	methods.Set("exp", &vm.GoFunc{Name: "math:exp", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		expValue := math.Exp(value)
		return []vm.Value{expValue}
	}})

	methods.Set("floor", &vm.GoFunc{Name: "math:floor", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		flooredValue := math.Floor(value)
		return []vm.Value{flooredValue}
	}})

	methods.Set("fmod", &vm.GoFunc{Name: "math:fmod", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		modValue := math.Mod(x, y)
		return []vm.Value{modValue}
	}})

	methods.Set("log", &vm.GoFunc{Name: "math:log", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("x", 1, args)
		logValue := math.Log(value)
		return []vm.Value{logValue}
	}})

	methods.Set("max", &vm.GoFunc{Name: "math:max", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		maxValue := math.Max(x, y)
		return []vm.Value{maxValue}
	}})

	methods.Set("min", &vm.GoFunc{Name: "math:min", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)
		minValue := math.Min(x, y)
		return []vm.Value{minValue}
	}})

	methods.Set("modf", &vm.GoFunc{Name: "math:modf", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		intPart, fracPart := math.Modf(value)
		return []vm.Value{intPart, fracPart}
	}})

	methods.Set("rad", &vm.GoFunc{Name: "math:rad", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		radValue := rad(value)
		return []vm.Value{radValue}
	}})

	methods.Set("tointeger", &vm.GoFunc{Name: "math:tointeger", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.AnyArg("tointeger", 1, args)
		integerValue, ok := vm.ToInteger(value)
		if !ok {
			panic(vm.Errorf("bad argument #1 to 'tointeger' (number expected)"))
		}
		return []vm.Value{integerValue}
	}})

	methods.Set("ult", &vm.GoFunc{Name: "math:ult", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("x", 1, args)
		y := vm.FloatArg("y", 2, args)

		returnValue := x < y
		return []vm.Value{returnValue}
	}})

	methods.Set("random", &vm.GoFunc{Name: "math:random", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		random := rand.Float64()
		return []vm.Value{random}
	}})

	methods.Set("sqrt", &vm.GoFunc{Name: "math:sqrt", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		value := vm.FloatArg("value", 1, args)
		sqrtValue := math.Sqrt(value)
		return []vm.Value{sqrtValue}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

func deg(x float64) float64 {
	return x * (180 / math.Pi)
}

func rad(x float64) float64 {
	return x * (math.Pi / 180)
}
