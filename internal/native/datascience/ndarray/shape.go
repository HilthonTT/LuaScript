// Package ndarray is a require()-able host module providing a dense,
// N-dimensional numeric array — the NumPy-style primitive that most
// data-science numerics build on. Unlike a Lua table (a boxed hash map),
// an ndarray stores its elements in a single contiguous []float64 in
// row-major (C) order, so vectorized arithmetic, reductions, and matrix
// products run over flat Go slices instead of chasing pointers.
//
// Construction:
//
//	local nd = require("ndarray")
//	a = nd.array({ {1, 2, 3}, {4, 5, 6} })   -- 2x3 from nested tables
//	z = nd.zeros(2, 3)                        -- 2x3 of zeros
//	r = nd.arange(0, 10)                      -- 0,1,..,9  (1-D)
//	l = nd.linspace(0, 1, 5)                  -- 5 points in [0,1]
//	i = nd.eye(3)                             -- 3x3 identity
//
// Arithmetic operators are overloaded and broadcast NumPy-style, so an
// array and a scalar, or two arrays whose shapes align, combine elementwise:
//
//	b = a * 2 + 1
//	c = a + nd.array({10, 20, 30})           -- row broadcast over a 2x3
//
// Reductions take an optional axis; with no axis they collapse to a scalar:
//
//	a:sum()          -- scalar
//	a:mean(1)        -- per-row means (a 1-D array of length 2)
//
// Every arithmetic/transform method returns a NEW array; the receiver is
// never mutated (only :set and the in-place-free API mutate, explicitly).
package ndarray

// Shape arithmetic: row-major strides, broadcasting, and shape formatting.

import (
	"strconv"
	"strings"
)

// rowStrides returns the row-major strides for a shape: strides[i] is the
// flat-index step for a unit increment along axis i.
func rowStrides(shape []int) []int {
	st := make([]int, len(shape))
	acc := 1
	for i := len(shape) - 1; i >= 0; i-- {
		st[i] = acc
		acc *= shape[i]
	}
	return st
}

// broadcast returns the shape two operands broadcast to, aligning axes from
// the right per NumPy rules. ok is false when the shapes are incompatible.
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

// bstrides returns per-axis flat-index steps for reading an operand of shape
// s while iterating over the broadcast shape out. Missing (leading) axes and
// size-1 axes get a stride of 0 so the operand's value repeats.
func bstrides(s, out []int) []int {
	n, m := len(out), len(s)
	real := rowStrides(s)
	st := make([]int, n)
	for i := 0; i < n; i++ {
		if i < n-m {
			continue // axis absent in operand -> stride 0
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
