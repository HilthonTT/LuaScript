package sort

import (
	"fmt"

	"github.com/hilthontt/luascript/vm"
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

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)

	return m
}

// sortDispatch is the single entry point shared by all sort methods.
// It extracts the array part of t into a typed Go slice, picks the
// matching concrete sort function via type assertion, and writes the
// sorted result back into t in place — matching Lua's table.sort
// semantics, where the caller's table is mutated and returned.
func sortDispatch(
	t *vm.Table,
	site string,
	intSort func([]int64) []int64,
	floatSort func([]float64) []float64,
	stringSort func([]string) []string,
) *vm.Table {
	arr, err := extractArray(t)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}

	switch a := arr.(type) {
	case []int64:
		writeBack(t, intSort(a))
	case []float64:
		writeBack(t, floatSort(a))
	case []string:
		writeBack(t, stringSort(a))
	}

	return t
}

// extractArray reads the sequential 1..n portion of t into a typed Go
// slice. The element type is inferred from the values: all-strings →
// []string, all-ints → []int64, mixed int/float (or all-float) →
// []float64. Mixing strings with numbers, or any other value type, is
// a hard error — sorting heterogeneous values has no obvious total
// order so we'd rather fail loudly than guess.
func extractArray(t *vm.Table) (any, error) {
	n := t.Len()
	if n == 0 {
		return []int64{}, nil
	}

	var hasInt, hasFloat, hasString bool

	for i := int64(1); i <= n; i++ {
		switch t.Get(i).(type) {
		case int64:
			hasInt = true
		case float64:
			hasFloat = true
		case string:
			hasString = true
		case nil:
			return nil, fmt.Errorf("nil value at index %d", i)
		default:
			return nil, fmt.Errorf("unsortable value at index %d", i)
		}
	}

	if hasString && (hasInt || hasFloat) {
		return nil, fmt.Errorf("cannot sort array mixing strings with numbers")
	}

	switch {
	case hasString:
		out := make([]string, n)
		for i := int64(1); i <= n; i++ {
			out[i-1] = t.Get(i).(string)
		}
		return out, nil
	case hasFloat:
		// Mixed int/float — promote ints to float so a single total
		// order applies. Note: int64 values above 2^53 lose precision
		// here, but that only matters if the user is mixing very
		// large ints with floats in the same array, which is already
		// a weird thing to do.
		out := make([]float64, n)
		for i := int64(1); i <= n; i++ {
			switch v := t.Get(i).(type) {
			case int64:
				out[i-1] = float64(v)
			case float64:
				out[i-1] = v
			}
		}
		return out, nil
	default:
		out := make([]int64, n)
		for i := int64(1); i <= n; i++ {
			out[i-1] = t.Get(i).(int64)
		}
		return out, nil
	}
}

// writeBack copies a sorted Go slice into the 1..n positions of t,
// matching Lua's in-place sort semantics. Indices beyond n are left
// untouched — callers shouldn't be putting holes in an array part
// anyway.
func writeBack[T any](t *vm.Table, values []T) {
	for i, v := range values {
		t.Set(int64(i+1), v)
	}
}
