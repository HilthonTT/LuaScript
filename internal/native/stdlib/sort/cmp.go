package sort

import (
	stdsort "sort"

	"github.com/hilthontt/luascript/internal/native"
	"github.com/hilthontt/luascript/internal/vm"
)

func cmpSortInPlace(v *vm.VM, t *vm.Table, cmp vm.Value, site string, stable bool) {
	requireCallable(cmp, site)

	n := int(t.Len())
	if n < 2 {
		return
	}

	snap := make([]vm.Value, n)
	for i := 0; i < n; i++ {
		snap[i] = t.Get(int64(i + 1))
	}

	less := func(i, j int) bool {
		r := v.CallValue(cmp, []vm.Value{snap[i], snap[j]}, 1)
		if len(r) == 0 {
			return false
		}
		return vm.IsTruthy(r[0])
	}

	if stable {
		stdsort.SliceStable(snap, less)
	} else {
		stdsort.Slice(snap, less)
	}

	for i := 0; i < n; i++ {
		t.Set(int64(i+1), snap[i])
	}
}

func cmpIsSorted(v *vm.VM, t *vm.Table, cmp vm.Value, site string) bool {
	requireCallable(cmp, site)
	n := int(t.Len())
	for i := 1; i < n; i++ {
		a := t.Get(int64(i))
		b := t.Get(int64(i + 1))
		r := v.CallValue(cmp, []vm.Value{b, a}, 1)
		if len(r) > 0 && vm.IsTruthy(r[0]) {
			return false
		}
	}
	return true
}

func stableDefault(t *vm.Table) {
	arr, err := native.ExtractArray(t)
	if err != nil {
		panic(vm.Errorf("sort.stable: %s", err.Error()))
	}
	switch a := arr.(type) {
	case []int64:
		stdsort.SliceStable(a, func(i, j int) bool {
			return a[i] < a[j]
		})
		native.WriteBack(t, a)
	case []float64:
		stdsort.SliceStable(a, func(i, j int) bool {
			return a[i] < a[j]
		})
		native.WriteBack(t, a)
	case []string:
		stdsort.SliceStable(a, func(i, j int) bool {
			return a[i] < a[j]
		})
		native.WriteBack(t, a)
	}
}

func defaultIsSorted(t *vm.Table) bool {
	n := t.Len()
	if n < 2 {
		return true
	}
	prev := t.Get(int64(1))
	for i := int64(2); i <= n; i++ {
		cur := t.Get(i)
		if !defaultLess(prev, cur) && !defaultEqual(prev, cur) {
			return false
		}
		prev = cur
	}
	return true
}

func defaultLess(a, b vm.Value) bool {
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		if !ok {
			panic(vm.Errorf("sort.is_sorted: cannot compare %s with %s", vm.TypeName(a), vm.TypeName(b)))
		}
		return x < y
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
	panic(vm.Errorf("sort.is_sorted: cannot compare %s with %s", vm.TypeName(a), vm.TypeName(b)))
}

func defaultEqual(a, b vm.Value) bool {
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case int64:
		switch y := b.(type) {
		case int64:
			return x == y
		case float64:
			return float64(x) == y
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return x == float64(y)
		case float64:
			return x == y
		}
	}
	return false
}

func requireCallable(v vm.Value, site string) {
	switch v.(type) {
	case *vm.Closure, *vm.GoFunc:
		return
	}
	panic(vm.Errorf("bad argument #2 to '%s' (function expected, got %s)", site, vm.TypeName(v)))
}
