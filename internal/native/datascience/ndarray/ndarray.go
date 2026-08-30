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

import (
	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterNDArrayPreload installs the loader under package.preload.
func RegisterNDArrayPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ndarray", ndLoader)
}

// ndarray is a dense N-dimensional array. data is the flat, row-major
// element buffer; shape[i] is the length of axis i. A 0-length shape is a
// scalar (one element) and is used internally to broadcast Lua numbers.
type ndarray struct {
	data  []float64
	shape []int
}

// Core numeric layer (VM-independent; exercised directly in the tests)

// maxNDElems bounds the element count of any single array. Script-supplied
// shapes must not be able to force an unrecoverable OOM (fatal, not
// pcall-catchable) or overflow the element-count product.
const maxNDElems = 1 << 26 // 64M elements ≈ 512 MiB of float64

func newND(shape []int) *ndarray {
	n := 1
	for _, d := range shape {
		if d < 0 {
			panic(vm.Errorf("ndarray: negative dimension %d", d))
		}
		if d > 0 && n > maxNDElems/d {
			panic(vm.Errorf("ndarray: shape exceeds %d elements", maxNDElems))
		}
		n *= d
	}
	return &ndarray{data: make([]float64, n), shape: append([]int(nil), shape...)}
}

func scalarND(x float64) *ndarray { return &ndarray{data: []float64{x}, shape: []int{}} }

func (a *ndarray) ndim() int { return len(a.shape) }
func (a *ndarray) size() int { return len(a.data) }
