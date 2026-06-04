package sort

// Comparator-driven sort helpers used by sort.sort(t, cmp), sort.stable,
// and sort.is_sorted. These live separately from the typed fast paths
// in sort.go so the "user supplied a Lua predicate" code path is easy
// to audit on its own.

import (
	stdsort "sort"

	"github.com/hilthontt/luascript/vm"
)

// cmpSortInPlace sorts t's 1..n array portion using `cmp` as the
// less-than predicate. cmp(a,b) is called from Go via v.CallValue; a
// truthy return means "a comes before b". A non-callable cmp raises
// with the usual bad-argument format.
//
// The sort works on a snapshot of the array so the in-flight comparator
// can read t (e.g. reach for a sibling field) without seeing partially
// written intermediate state. Once the snapshot is sorted, it's written
// back to t in one pass.
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

// cmpIsSorted walks adjacent pairs and asks the user's cmp whether the
// later element should precede the earlier — if it ever should, the
// array is not sorted. Empty / one-element arrays are trivially sorted.
func cmpIsSorted(v *vm.VM, t *vm.Table, cmp vm.Value, site string) bool {
	requireCallable(cmp, site)
	n := int(t.Len())
	for i := 1; i < n; i++ {
		a := t.Get(int64(i))
		b := t.Get(int64(i + 1))
		// "out of order" iff cmp(b, a) is true — i.e. b should come
		// before a even though it's after it in the array.
		r := v.CallValue(cmp, []vm.Value{b, a}, 1)
		if len(r) > 0 && vm.IsTruthy(r[0]) {
			return false
		}
	}
	return true
}

// stableDefault is the no-cmp branch of sort.stable. It extracts the
// array into the same typed slice the algorithm-specific sorts use and
// runs Go's stdsort.Slice stable. We don't reuse sortDispatch because
// that path is hard-wired to non-stable algorithms; the duplication is
// small and easier to follow than threading a stable-flag through.
func stableDefault(t *vm.Table) {
	arr, err := extractArray(t)
	if err != nil {
		panic(vm.Errorf("sort.stable: %s", err.Error()))
	}
	switch a := arr.(type) {
	case []int64:
		stdsort.SliceStable(a, func(i, j int) bool { return a[i] < a[j] })
		writeBack(t, a)
	case []float64:
		stdsort.SliceStable(a, func(i, j int) bool { return a[i] < a[j] })
		writeBack(t, a)
	case []string:
		stdsort.SliceStable(a, func(i, j int) bool { return a[i] < a[j] })
		writeBack(t, a)
	}
}

// defaultIsSorted is the no-cmp branch of sort.is_sorted. It avoids
// pulling the whole array into a typed slice — for the in-order case
// that's a wasted allocation when we only need to compare neighbours.
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

// defaultLess and defaultEqual mirror the type rules of extractArray:
// strings compare lexically, numbers compare numerically (with int/float
// promoted to float64 for the mixed case), and anything else is an error.
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

// requireCallable rejects a non-function cmp early with the standard
// bad-argument format. Without this, the first failing comparison would
// surface as a confusing "attempt to call a <type> value" from inside
// the sort.
func requireCallable(v vm.Value, site string) {
	switch v.(type) {
	case *vm.Closure, *vm.GoFunc:
		return
	}
	panic(vm.Errorf("bad argument #2 to '%s' (function expected, got %s)", site, vm.TypeName(v)))
}
