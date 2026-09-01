package ndarray

import (
	"sync"

	"github.com/hilthontt/luascript/internal/vm"
)

const ndKey = "\x00ndarray"

var (
	ndMeta    *vm.Table
	metaOnce  sync.Once
	ndMethods *vm.Table
)

func wrap(a *ndarray) *vm.Table {
	metaOnce.Do(buildMeta)
	t := vm.NewTable(0, 1)
	t.Set(ndKey, a)
	t.SetMetatable(ndMeta)
	return t
}

func asND(v vm.Value) (*ndarray, bool) {
	switch x := v.(type) {
	case int64:
		return scalarND(float64(x)), true
	case float64:
		return scalarND(x), true
	case *vm.Table:
		if p, ok := x.Get(ndKey).(*ndarray); ok {
			return p, true
		}
	}
	return nil, false
}

func ndArg(site string, v vm.Value) *ndarray {
	if a, ok := asND(v); ok {
		return a
	}
	panic(vm.Errorf("%s: expected an ndarray or number, got %s", site, vm.TypeName(v)))
}

func selfND(site string, args []vm.Value) *ndarray {
	if len(args) == 0 {
		panic(vm.Errorf("%s: called without a receiver (use a:method(), not a.method())", site))
	}
	t, ok := args[0].(*vm.Table)
	if !ok {
		panic(vm.Errorf("%s: receiver is not an ndarray", site))
	}
	p, ok := t.Get(ndKey).(*ndarray)
	if !ok {
		panic(vm.Errorf("%s: receiver is not an ndarray", site))
	}
	return p
}

func argAt(args []vm.Value, i int) vm.Value {
	if i < 1 || i > len(args) {
		return nil
	}
	return args[i-1]
}

func ndLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 16)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		m.Set(name, &vm.GoFunc{Name: "ndarray." + name, Fn: fn})
	}

	set("array", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(fromNested("ndarray.array", vm.TableArg("ndarray.array", 1, args)))}
	})
	set("zeros", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(newND(shapeArgs("ndarray.zeros", args, 1)))}
	})
	set("ones", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := newND(shapeArgs("ndarray.ones", args, 1))
		for i := range a.data {
			a.data[i] = 1
		}
		return []vm.Value{wrap(a)}
	})
	set("full", func(_ *vm.VM, args []vm.Value) []vm.Value {
		val := vm.FloatArg("ndarray.full", 1, args)
		a := newND(shapeArgs("ndarray.full", args, 2))
		for i := range a.data {
			a.data[i] = val
		}
		return []vm.Value{wrap(a)}
	})
	set("arange", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(arange(args))}
	})
	set("linspace", func(_ *vm.VM, args []vm.Value) []vm.Value {
		lo := vm.FloatArg("ndarray.linspace", 1, args)
		hi := vm.FloatArg("ndarray.linspace", 2, args)
		n := int(vm.IntArg("ndarray.linspace", 3, args))
		if n < 0 {
			panic(vm.Errorf("ndarray.linspace: count must be >= 0"))
		}
		a := newND([]int{n})
		if n == 1 {
			a.data[0] = lo
		} else {
			step := (hi - lo) / float64(n-1)
			for i := 0; i < n; i++ {
				a.data[i] = lo + step*float64(i)
			}
		}
		return []vm.Value{wrap(a)}
	})
	set("eye", func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := int(vm.IntArg("ndarray.eye", 1, args))
		a := newND([]int{n, n})
		for i := 0; i < n; i++ {
			a.data[i*n+i] = 1
		}
		return []vm.Value{wrap(a)}
	})
	set("from_table", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(fromNested("ndarray.from_table", vm.TableArg("ndarray.from_table", 1, args)))}
	})
	set("matmul", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := ndArg("ndarray.matmul", argAt(args, 1))
		b := ndArg("ndarray.matmul", argAt(args, 2))
		return []vm.Value{result(matmul(a, b))}
	})
	set("concat", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := ndArg("ndarray.concat", argAt(args, 1))
		b := ndArg("ndarray.concat", argAt(args, 2))
		return []vm.Value{wrap(concat1D("ndarray.concat", a, b))}
	})
	set("is_ndarray", func(_ *vm.VM, args []vm.Value) []vm.Value {
		if t, ok := argAt(args, 1).(*vm.Table); ok {
			if _, ok := t.Get(ndKey).(*ndarray); ok {
				return []vm.Value{true}
			}
		}
		return []vm.Value{false}
	})

	m.Set("VERSION", "0.1.0")
	return []vm.Value{m}
}

func shapeArgs(site string, args []vm.Value, from int) []int {
	if t, ok := argAt(args, from).(*vm.Table); ok {
		n := int(t.Len())
		shape := make([]int, n)
		for i := 1; i <= n; i++ {
			shape[i-1] = int(vm.IntArg(site, 1, []vm.Value{t.Get(int64(i))}))
		}
		return shape
	}
	shape := []int{}
	for i := from; i <= len(args); i++ {
		shape = append(shape, int(vm.IntArg(site, i, args)))
	}
	if len(shape) == 0 {
		panic(vm.Errorf("%s: expected at least one dimension", site))
	}
	return shape
}

func arange(args []vm.Value) *ndarray {
	var start, stop, step float64 = 0, 0, 1
	switch len(args) {
	case 1:
		stop = vm.FloatArg("ndarray.arange", 1, args)
	case 2:
		start = vm.FloatArg("ndarray.arange", 1, args)
		stop = vm.FloatArg("ndarray.arange", 2, args)
	default:
		start = vm.FloatArg("ndarray.arange", 1, args)
		stop = vm.FloatArg("ndarray.arange", 2, args)
		step = vm.FloatArg("ndarray.arange", 3, args)
	}
	if step == 0 {
		panic(vm.Errorf("ndarray.arange: step must be non-zero"))
	}
	var vals []float64
	if step > 0 {
		for x := start; x < stop; x += step {
			vals = append(vals, x)
		}
	} else {
		for x := start; x > stop; x += step {
			vals = append(vals, x)
		}
	}
	a := newND([]int{len(vals)})
	copy(a.data, vals)
	return a
}

func concat1D(site string, a, b *ndarray) *ndarray {
	if a.ndim() != 1 || b.ndim() != 1 {
		panic(vm.Errorf("%s: concat currently supports 1-D arrays only", site))
	}
	r := newND([]int{a.size() + b.size()})
	copy(r.data, a.data)
	copy(r.data[a.size():], b.data)
	return r
}

func fromNested(site string, t *vm.Table) *ndarray {
	shape := inferShape(t)
	a := newND(shape)
	idx := 0
	var fill func(v vm.Value, depth int)
	fill = func(v vm.Value, depth int) {
		if depth == len(shape) {
			f, ok := vm.ToFloat(v)
			if !ok {
				panic(vm.Errorf("%s: non-numeric element %s", site, vm.TypeName(v)))
			}
			a.data[idx] = f
			idx++
			return
		}
		tv, ok := v.(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: ragged nesting — expected a sub-array at depth %d", site, depth))
		}
		if int(tv.Len()) != shape[depth] {
			panic(vm.Errorf("%s: ragged array — axis %d has inconsistent length", site, depth))
		}
		for i := 1; i <= shape[depth]; i++ {
			fill(tv.Get(int64(i)), depth+1)
		}
	}
	fill(t, 0)
	return a
}

func inferShape(t *vm.Table) []int {
	shape := []int{}
	var v vm.Value = t
	for {
		tv, ok := v.(*vm.Table)
		if !ok {
			break
		}
		n := int(tv.Len())
		shape = append(shape, n)
		if n == 0 {
			break
		}
		v = tv.Get(int64(1))
	}
	return shape
}

func result(a *ndarray) vm.Value {
	if a.ndim() == 0 {
		return a.data[0]
	}
	return wrap(a)
}
