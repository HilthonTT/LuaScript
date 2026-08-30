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

// NumPy-flavored rendering with edge truncation for large arrays.

import (
	"math"
	"strconv"
	"strings"
)

// Rendering (NumPy-flavored, with edge truncation for large arrays)

const edgeItems = 3 // show first/last N along any axis longer than 2*edgeItems

func fmtNum(x float64) string {
	if x == math.Trunc(x) && !math.IsInf(x, 0) && math.Abs(x) < 1e15 {
		return strconv.FormatInt(int64(x), 10)
	}
	return strconv.FormatFloat(x, 'g', 6, 64)
}

func (a *ndarray) render() string {
	if a.ndim() == 0 {
		return "ndarray(" + fmtNum(a.data[0]) + ")"
	}
	// Column width for alignment across the whole (possibly truncated) array.
	width := 0
	for _, x := range a.data {
		if l := len(fmtNum(x)); l > width {
			width = l
		}
	}
	strides := rowStrides(a.shape)
	var render func(axis, base int) string
	render = func(axis, base int) string {
		n := a.shape[axis]
		if axis == len(a.shape)-1 {
			var sb strings.Builder
			sb.WriteByte('[')
			writeCell := func(k int) {
				s := fmtNum(a.data[base+k*strides[axis]])
				sb.WriteString(strings.Repeat(" ", width-len(s)))
				sb.WriteString(s)
			}
			if n > 2*edgeItems {
				for k := 0; k < edgeItems; k++ {
					if k > 0 {
						sb.WriteByte(' ')
					}
					writeCell(k)
				}
				sb.WriteString(" ... ")
				for k := n - edgeItems; k < n; k++ {
					if k > n-edgeItems {
						sb.WriteByte(' ')
					}
					writeCell(k)
				}
			} else {
				for k := 0; k < n; k++ {
					if k > 0 {
						sb.WriteByte(' ')
					}
					writeCell(k)
				}
			}
			sb.WriteByte(']')
			return sb.String()
		}
		indent := strings.Repeat(" ", axis+1)
		sep := ",\n" + strings.Repeat("\n", len(a.shape)-axis-2) + indent
		rows := func(k int) string { return render(axis+1, base+k*strides[axis]) }
		var parts []string
		if n > 2*edgeItems {
			for k := 0; k < edgeItems; k++ {
				parts = append(parts, rows(k))
			}
			parts = append(parts, "...")
			for k := n - edgeItems; k < n; k++ {
				parts = append(parts, rows(k))
			}
		} else {
			for k := 0; k < n; k++ {
				parts = append(parts, rows(k))
			}
		}
		return "[" + strings.Join(parts, sep) + "]"
	}
	return render(0, 0)
}
