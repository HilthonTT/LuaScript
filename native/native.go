package native

import (
	"fmt"

	"github.com/hilthontt/luascript/vm"
)

// extractArray reads the sequential 1..n portion of t into a typed Go
// slice. The element type is inferred from the values: all-strings →
// []string, all-ints → []int64, mixed int/float (or all-float) →
// []float64. Mixing strings with numbers, or any other value type, is
// a hard error — sorting heterogeneous values has no obvious total
// order so we'd rather fail loudly than guess.
func ExtractArray(t *vm.Table) (any, error) {
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
func WriteBack[T any](t *vm.Table, values []T) {
	for i, v := range values {
		t.Set(int64(i+1), v)
	}
}
