package sort

import (
	"github.com/hilthontt/luascript/internal/native"
	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterSortPreload(v *vm.VM) {
	vm.RegisterPreload(v, "sort", sortLoader)
}

func sortLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newSort()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newSort() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 4)

	methods.Set("quicksort", &vm.GoFunc{Name: "sort:quicksort", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.quicksort", 1, args)
		return []vm.Value{sortDispatch(t, "sort.quicksort", Quicksort[int64], Quicksort[float64], Quicksort[string])}
	}})

	methods.Set("bubble", &vm.GoFunc{Name: "sort:bubble", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.bubble", 1, args)
		return []vm.Value{sortDispatch(t, "sort.bubble", Bubble[int64], Bubble[float64], Bubble[string])}
	}})

	methods.Set("circle", &vm.GoFunc{Name: "sort:circle", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.circle", 1, args)
		return []vm.Value{sortDispatch(t, "sort.circle", Circle[int64], Circle[float64], Circle[string])}
	}})

	methods.Set("simple", &vm.GoFunc{Name: "sort:simple", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.simple", 1, args)
		return []vm.Value{sortDispatch(t, "sort.simple", Simple[int64], Simple[float64], Simple[string])}
	}})

	methods.Set("sort", &vm.GoFunc{Name: "sort:sort", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.sort", 1, args)
		if len(args) >= 2 && args[1] != nil {
			cmpSortInPlace(v, t, args[1], "sort.sort", false)
			return []vm.Value{t}
		}
		return []vm.Value{sortDispatch(t, "sort.sort", Quicksort[int64], Quicksort[float64], Quicksort[string])}
	}})

	methods.Set("stable", &vm.GoFunc{Name: "sort:stable", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.stable", 1, args)
		if len(args) >= 2 && args[1] != nil {
			cmpSortInPlace(v, t, args[1], "sort.stable", true)
			return []vm.Value{t}
		}
		stableDefault(t)
		return []vm.Value{t}
	}})

	methods.Set("reverse", &vm.GoFunc{Name: "sort:reverse", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.reverse", 1, args)
		n := t.Len()
		for i, j := int64(1), n; i < j; i, j = i+1, j-1 {
			a, b := t.Get(i), t.Get(j)
			t.Set(i, b)
			t.Set(j, a)
		}
		return []vm.Value{t}
	}})

	methods.Set("is_sorted", &vm.GoFunc{Name: "sort:is_sorted", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("sort.is_sorted", 1, args)
		if len(args) >= 2 && args[1] != nil {
			return []vm.Value{cmpIsSorted(v, t, args[1], "sort.is_sorted")}
		}
		return []vm.Value{defaultIsSorted(t)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)

	return m
}

func sortDispatch(
	t *vm.Table,
	site string,
	intSort func([]int64) []int64,
	floatSort func([]float64) []float64,
	stringSort func([]string) []string,
) *vm.Table {
	arr, err := native.ExtractArray(t)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}

	switch a := arr.(type) {
	case []int64:
		native.WriteBack(t, intSort(a))
	case []float64:
		native.WriteBack(t, floatSort(a))
	case []string:
		native.WriteBack(t, stringSort(a))
	}

	return t
}
