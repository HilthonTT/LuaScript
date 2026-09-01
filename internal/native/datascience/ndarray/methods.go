package ndarray

import (
	"fmt"
	"math"
	"sort"

	"github.com/hilthontt/luascript/internal/vm"
)

func buildMeta() {
	ndMethods = vm.NewTable(0, 32)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		ndMethods.Set(name, &vm.GoFunc{Name: "ndarray:" + name, Fn: fn})
	}

	set("shape", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:shape", args)
		t := vm.NewTable(len(a.shape), 0)
		for i, d := range a.shape {
			t.Set(int64(i+1), int64(d))
		}
		return []vm.Value{t}
	})
	set("ndim", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{int64(selfND("ndarray:ndim", args).ndim())}
	})
	set("size", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{int64(selfND("ndarray:size", args).size())}
	})

	set("get", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:get", args)
		return []vm.Value{a.data[a.flatIndex("ndarray:get", args[1:])]}
	})
	set("set", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:set", args)
		val := vm.FloatArg("ndarray:set", 2, args)
		a.data[a.flatIndex("ndarray:set", args[2:])] = val
		return nil
	})

	set("reshape", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:reshape", args)
		return []vm.Value{wrap(a.reshape("ndarray:reshape", shapeArgs("ndarray:reshape", args, 2)))}
	})
	set("flatten", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:flatten", args)
		return []vm.Value{wrap(a.reshape("ndarray:flatten", []int{a.size()}))}
	})
	set("transpose", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:transpose", args)
		var perm []int
		if len(args) > 1 {
			perm = shapeArgs("ndarray:transpose", args, 2)
		}
		return []vm.Value{wrap(a.transpose("ndarray:transpose", perm))}
	})
	set("copy", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:copy", args)
		return []vm.Value{wrap(&ndarray{data: append([]float64(nil), a.data...), shape: append([]int(nil), a.shape...)})}
	})

	reduction := func(name string, all func([]float64) float64) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := selfND("ndarray:"+name, args)
			if ax := argAt(args, 2); ax != nil {
				axis := int(vm.IntArg("ndarray:"+name, 2, args)) - 1
				return []vm.Value{result(a.alongAxis("ndarray:"+name, axis, all))}
			}
			return []vm.Value{a.reduceAll(all)}
		})
	}
	reduction("sum", sumf)
	reduction("mean", meanf)
	reduction("prod", prodf)
	reduction("max", maxf)
	reduction("min", minf)
	reduction("std", stdf)
	reduction("var", varf)

	set("argmax", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:argmax", args)
		return []vm.Value{int64(argExtreme(a.data, true) + 1)}
	})
	set("argmin", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:argmin", args)
		return []vm.Value{int64(argExtreme(a.data, false) + 1)}
	})

	unary := func(name string, f func(float64) float64) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			return []vm.Value{wrap(selfND("ndarray:"+name, args).unary(f))}
		})
	}
	unary("abs", math.Abs)
	unary("exp", math.Exp)
	unary("log", math.Log)
	unary("sqrt", math.Sqrt)
	unary("sin", math.Sin)
	unary("cos", math.Cos)
	unary("tanh", math.Tanh)
	unary("floor", math.Floor)
	unary("ceil", math.Ceil)
	unary("neg", func(x float64) float64 { return -x })
	unary("log2", math.Log2)
	unary("log10", math.Log10)
	unary("sign", func(x float64) float64 {
		switch {
		case x > 0:
			return 1
		case x < 0:
			return -1
		default:
			return 0
		}
	})
	unary("round", math.Round)
	unary("sinh", math.Sinh)
	unary("cosh", math.Cosh)
	unary("asin", math.Asin)
	unary("acos", math.Acos)
	unary("atan", math.Atan)

	set("sort", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:sort", args)
		out := append([]float64(nil), a.data...)
		sort.Float64s(out)
		return []vm.Value{wrap(&ndarray{data: out, shape: []int{len(out)}})}
	})

	set("argsort", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:argsort", args)
		idx := make([]int, len(a.data))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(i, j int) bool { return a.data[idx[i]] < a.data[idx[j]] })
		out := make([]float64, len(idx))
		for i, v := range idx {
			out[i] = float64(v + 1)
		}
		return []vm.Value{wrap(&ndarray{data: out, shape: []int{len(out)}})}
	})

	set("median", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:median", args)
		if len(a.data) == 0 {
			panic(vm.Errorf("ndarray:median: array is empty"))
		}
		s := append([]float64(nil), a.data...)
		sort.Float64s(s)
		mid := len(s) / 2
		if len(s)%2 == 1 {
			return []vm.Value{s[mid]}
		}
		return []vm.Value{(s[mid-1] + s[mid]) / 2}
	})

	set("cumsum", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:cumsum", args)
		out := make([]float64, len(a.data))
		run := 0.0
		for i, x := range a.data {
			run += x
			out[i] = run
		}
		return []vm.Value{wrap(&ndarray{data: out, shape: []int{len(out)}})}
	})
	set("diff", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:diff", args)
		if len(a.data) < 2 {
			return []vm.Value{wrap(&ndarray{data: []float64{}, shape: []int{0}})}
		}
		out := make([]float64, len(a.data)-1)
		for i := 1; i < len(a.data); i++ {
			out[i-1] = a.data[i] - a.data[i-1]
		}
		return []vm.Value{wrap(&ndarray{data: out, shape: []int{len(out)}})}
	})

	set("any", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:any", args)
		for _, x := range a.data {
			if x != 0 {
				return []vm.Value{true}
			}
		}
		return []vm.Value{false}
	})
	set("all", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:all", args)
		for _, x := range a.data {
			if x == 0 {
				return []vm.Value{false}
			}
		}
		return []vm.Value{true}
	})

	set("nonzero", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:nonzero", args)
		var out []float64
		for i, x := range a.data {
			if x != 0 {
				out = append(out, float64(i+1))
			}
		}
		return []vm.Value{wrap(&ndarray{data: out, shape: []int{len(out)}})}
	})

	set("count_nonzero", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:count_nonzero", args)
		n := int64(0)
		for _, x := range a.data {
			if x != 0 {
				n++
			}
		}
		return []vm.Value{n}
	})

	set("pow", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:pow", args)
		e := vm.FloatArg("ndarray:pow", 2, args)
		return []vm.Value{wrap(a.unary(func(x float64) float64 { return math.Pow(x, e) }))}
	})
	set("clip", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:clip", args)
		lo := vm.FloatArg("ndarray:clip", 2, args)
		hi := vm.FloatArg("ndarray:clip", 3, args)
		return []vm.Value{wrap(a.unary(func(x float64) float64 {
			switch {
			case x < lo:
				return lo
			case x > hi:
				return hi
			default:
				return x
			}
		}))}
	})

	binary := func(name string, op func(x, y float64) float64) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := selfND("ndarray:"+name, args)
			b := ndArg("ndarray:"+name, argAt(args, 2))
			return []vm.Value{wrap(ewise("ndarray:"+name, a, b, op))}
		})
	}
	binary("add", func(x, y float64) float64 { return x + y })
	binary("sub", func(x, y float64) float64 { return x - y })
	binary("mul", func(x, y float64) float64 { return x * y })
	binary("div", func(x, y float64) float64 { return x / y })

	set("dot", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:dot", args)
		b := ndArg("ndarray:dot", argAt(args, 2))
		return []vm.Value{result(matmul(a, b))}
	})
	set("matmul", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:matmul", args)
		b := ndArg("ndarray:matmul", argAt(args, 2))
		return []vm.Value{result(matmul(a, b))}
	})

	set("map", func(v *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:map", args)
		fn := argAt(args, 2)
		switch fn.(type) {
		case *vm.Closure, *vm.GoFunc:
		default:
			panic(vm.Errorf("ndarray:map: argument must be a function, got %s", vm.TypeName(fn)))
		}
		r := &ndarray{data: make([]float64, len(a.data)), shape: append([]int(nil), a.shape...)}
		for i, x := range a.data {
			res := v.CallValue(fn, []vm.Value{x, int64(i + 1)}, 1)
			if len(res) > 0 {
				f, ok := vm.ToFloat(res[0])
				if !ok {
					panic(vm.Errorf("ndarray:map: function must return a number, got %s", vm.TypeName(res[0])))
				}
				r.data[i] = f
			}
		}
		return []vm.Value{wrap(r)}
	})
	set("to_table", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{selfND("ndarray:to_table", args).toTable()}
	})
	set("tolist", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:tolist", args)
		t := vm.NewTable(len(a.data), 0)
		for i, x := range a.data {
			t.Set(int64(i+1), x)
		}
		return []vm.Value{t}
	})
	set("show", func(_ *vm.VM, args []vm.Value) []vm.Value {
		fmt.Println(selfND("ndarray:show", args).render())
		return nil
	})

	ndMeta = vm.NewTable(0, 16)
	ndMeta.Set("__index", ndMethods)
	ndMeta.Set("__tostring", &vm.GoFunc{Name: "ndarray:__tostring", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{selfND("ndarray:__tostring", args).render()}
	}})
	ndMeta.Set("__len", &vm.GoFunc{Name: "ndarray:__len", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:__len", args)
		if a.ndim() == 0 {
			return []vm.Value{int64(1)}
		}
		return []vm.Value{int64(a.shape[0])}
	}})
	ndMeta.Set("__eq", &vm.GoFunc{Name: "ndarray:__eq", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := ndArg("ndarray:__eq", argAt(args, 1))
		b := ndArg("ndarray:__eq", argAt(args, 2))
		return []vm.Value{a.equal(b)}
	}})
	op := func(event, site string, f func(x, y float64) float64) {
		ndMeta.Set(event, &vm.GoFunc{Name: "ndarray:" + event, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := ndArg(site, argAt(args, 1))
			b := ndArg(site, argAt(args, 2))
			return []vm.Value{wrap(ewise(site, a, b, f))}
		}})
	}
	op("__add", "ndarray:+", func(x, y float64) float64 { return x + y })
	op("__sub", "ndarray:-", func(x, y float64) float64 { return x - y })
	op("__mul", "ndarray:*", func(x, y float64) float64 { return x * y })
	op("__div", "ndarray:/", func(x, y float64) float64 { return x / y })
	op("__mod", "ndarray:%", math.Mod)
	op("__pow", "ndarray:^", math.Pow)
	ndMeta.Set("__unm", &vm.GoFunc{Name: "ndarray:__unm", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(ndArg("ndarray:__unm", argAt(args, 1)).unary(func(x float64) float64 { return -x }))}
	}})
}

func argExtreme(xs []float64, wantMax bool) int {
	if len(xs) == 0 {
		panic(vm.Errorf("ndarray: arg reduction of an empty array"))
	}
	best := 0
	for i, x := range xs {
		if (wantMax && x > xs[best]) || (!wantMax && x < xs[best]) {
			best = i
		}
	}
	return best
}

func (a *ndarray) flatIndex(site string, idxArgs []vm.Value) int {
	if len(idxArgs) != len(a.shape) {
		panic(vm.Errorf("%s: expected %d indices for a %d-D array, got %d",
			site, len(a.shape), len(a.shape), len(idxArgs)))
	}
	strides := rowStrides(a.shape)
	off := 0
	for i, v := range idxArgs {
		ix, ok := vm.ToInteger(v)
		if !ok {
			panic(vm.Errorf("%s: index %d is not an integer", site, i+1))
		}
		k := int(ix)
		if k < 1 || k > a.shape[i] {
			panic(vm.Errorf("%s: index %d out of range [1, %d] on axis %d", site, k, a.shape[i], i+1))
		}
		off += (k - 1) * strides[i]
	}
	return off
}

func (a *ndarray) equal(b *ndarray) bool {
	if len(a.shape) != len(b.shape) {
		return false
	}
	for i := range a.shape {
		if a.shape[i] != b.shape[i] {
			return false
		}
	}
	for i := range a.data {
		if a.data[i] != b.data[i] {
			return false
		}
	}
	return true
}

func (a *ndarray) toTable() vm.Value {
	if a.ndim() == 0 {
		return a.data[0]
	}
	var build func(shape []int, data []float64) *vm.Table
	build = func(shape []int, data []float64) *vm.Table {
		t := vm.NewTable(shape[0], 0)
		if len(shape) == 1 {
			for i := 0; i < shape[0]; i++ {
				t.Set(int64(i+1), data[i])
			}
			return t
		}
		stride := len(data) / shape[0]
		for i := 0; i < shape[0]; i++ {
			t.Set(int64(i+1), build(shape[1:], data[i*stride:(i+1)*stride]))
		}
		return t
	}
	return build(a.shape, a.data)
}
