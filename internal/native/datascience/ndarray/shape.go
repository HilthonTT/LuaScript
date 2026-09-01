package ndarray

import (
	"strconv"
	"strings"
)

func rowStrides(shape []int) []int {
	st := make([]int, len(shape))
	acc := 1
	for i := len(shape) - 1; i >= 0; i-- {
		st[i] = acc
		acc *= shape[i]
	}
	return st
}

func broadcast(a, b []int) (out []int, ok bool) {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out = make([]int, n)
	for i := 0; i < n; i++ {
		ai, bi := 1, 1
		if i >= n-len(a) {
			ai = a[i-(n-len(a))]
		}
		if i >= n-len(b) {
			bi = b[i-(n-len(b))]
		}
		switch {
		case ai == bi:
			out[i] = ai
		case ai == 1:
			out[i] = bi
		case bi == 1:
			out[i] = ai
		default:
			return nil, false
		}
	}
	return out, true
}

func bstrides(s, out []int) []int {
	n, m := len(out), len(s)
	real := rowStrides(s)
	st := make([]int, n)
	for i := 0; i < n; i++ {
		if i < n-m {
			continue
		}
		if s[i-(n-m)] != 1 {
			st[i] = real[i-(n-m)]
		}
	}
	return st
}

func shapeStr(shape []int) string {
	parts := make([]string, len(shape))
	for i, d := range shape {
		parts[i] = strconv.Itoa(d)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
