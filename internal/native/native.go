package native

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/vm"
)

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

func WriteBack[T any](t *vm.Table, values []T) {
	for i, v := range values {
		t.Set(int64(i+1), v)
	}
}

func Clamp(x, min, max float64) float64 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}
