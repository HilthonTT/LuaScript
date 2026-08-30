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

// Array operations: elementwise, reductions, reshape/transpose, matmul.

import (
	"math"

	"github.com/hilthontt/luascript/internal/vm"
)

// ewise applies a binary op elementwise with broadcasting.
func ewise(site string, a, b *ndarray, op func(x, y float64) float64) *ndarray {
	out, ok := broadcast(a.shape, b.shape)
	if !ok {
		panic(vm.Errorf("%s: operands could not be broadcast together with shapes %s and %s",
			site, shapeStr(a.shape), shapeStr(b.shape)))
	}
	r := newND(out)
	sa := bstrides(a.shape, out)
	sb := bstrides(b.shape, out)
	idx := make([]int, len(out))
	oa, ob := 0, 0
	for i := range r.data {
		r.data[i] = op(a.data[oa], b.data[ob])
		// Odometer increment of the multi-index, keeping oa/ob in step.
		for d := len(out) - 1; d >= 0; d-- {
			idx[d]++
			oa += sa[d]
			ob += sb[d]
			if idx[d] < out[d] {
				break
			}
			idx[d] = 0
			oa -= sa[d] * out[d]
			ob -= sb[d] * out[d]
		}
	}
	return r
}

// unary applies f elementwise, returning a new array of the same shape.
func (a *ndarray) unary(f func(float64) float64) *ndarray {
	r := &ndarray{data: make([]float64, len(a.data)), shape: append([]int(nil), a.shape...)}
	for i, x := range a.data {
		r.data[i] = f(x)
	}
	return r
}

// alongAxis reduces one axis with reducer, returning an array with that axis
// removed. axis is 0-based.
func (a *ndarray) alongAxis(site string, axis int, reducer func([]float64) float64) *ndarray {
	if axis < 0 || axis >= len(a.shape) {
		panic(vm.Errorf("%s: axis %d out of range for %d-D array", site, axis+1, len(a.shape)))
	}
	outShape := make([]int, 0, len(a.shape)-1)
	for i, d := range a.shape {
		if i != axis {
			outShape = append(outShape, d)
		}
	}
	r := newND(outShape)
	strides := rowStrides(a.shape)
	axisLen, axisStride := a.shape[axis], strides[axis]
	idx := make([]int, len(outShape)) // index into outShape
	buf := make([]float64, axisLen)
	for o := range r.data {
		// Map the output multi-index to a base offset in a (axis fixed at 0).
		base, oi := 0, 0
		for src := 0; src < len(a.shape); src++ {
			if src == axis {
				continue
			}
			base += idx[oi] * strides[src]
			oi++
		}
		for k := 0; k < axisLen; k++ {
			buf[k] = a.data[base+k*axisStride]
		}
		r.data[o] = reducer(buf)
		for d := len(outShape) - 1; d >= 0; d-- {
			idx[d]++
			if idx[d] < outShape[d] {
				break
			}
			idx[d] = 0
		}
	}
	return r
}

// reduceAll collapses every element with reducer to a single value.
func (a *ndarray) reduceAll(reducer func([]float64) float64) float64 {
	return reducer(a.data)
}

func sumf(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}
func meanf(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	return sumf(xs) / float64(len(xs))
}
func prodf(xs []float64) float64 {
	p := 1.0
	for _, x := range xs {
		p *= x
	}
	return p
}
func maxf(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
func minf(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}
func varf(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	m := meanf(xs)
	s := 0.0
	for _, x := range xs {
		d := x - m
		s += d * d
	}
	return s / float64(len(xs))
}
func stdf(xs []float64) float64 { return math.Sqrt(varf(xs)) }

// reshape returns a new (copied) array with the given shape, which must have
// the same total size. A single -1 dimension is inferred.
func (a *ndarray) reshape(site string, shape []int) *ndarray {
	total := 1
	infer := -1
	for i, d := range shape {
		if d == -1 {
			if infer >= 0 {
				panic(vm.Errorf("%s: can only infer one dimension", site))
			}
			infer = i
			continue
		}
		if d < 0 {
			panic(vm.Errorf("%s: invalid dimension %d", site, d))
		}
		total *= d
	}
	shape = append([]int(nil), shape...)
	if infer >= 0 {
		if total == 0 || a.size()%total != 0 {
			panic(vm.Errorf("%s: cannot infer dimension for size %d", site, a.size()))
		}
		shape[infer] = a.size() / total
		total *= shape[infer]
	}
	if total != a.size() {
		panic(vm.Errorf("%s: cannot reshape array of size %d into %s", site, a.size(), shapeStr(shape)))
	}
	r := newND(shape)
	copy(r.data, a.data)
	return r
}

// transpose returns a new array with axes permuted by perm (a permutation of
// 0..ndim-1). A nil perm reverses the axes.
func (a *ndarray) transpose(site string, perm []int) *ndarray {
	nd := len(a.shape)
	if perm == nil {
		perm = make([]int, nd)
		for i := range perm {
			perm[i] = nd - 1 - i
		}
	}
	if len(perm) != nd {
		panic(vm.Errorf("%s: permutation has %d axes, array has %d", site, len(perm), nd))
	}
	seen := make([]bool, nd)
	outShape := make([]int, nd)
	for i, p := range perm {
		if p < 0 || p >= nd || seen[p] {
			panic(vm.Errorf("%s: invalid axis permutation", site))
		}
		seen[p] = true
		outShape[i] = a.shape[p]
	}
	r := newND(outShape)
	inStr := rowStrides(a.shape)
	idx := make([]int, nd)
	for o := range r.data {
		off := 0
		for i := 0; i < nd; i++ {
			off += idx[i] * inStr[perm[i]]
		}
		r.data[o] = a.data[off]
		for d := nd - 1; d >= 0; d-- {
			idx[d]++
			if idx[d] < outShape[d] {
				break
			}
			idx[d] = 0
		}
	}
	return r
}

// matmul implements dot/matrix-product for 1-D and 2-D operands, returning a
// scalar (as a 0-D array) for the vector·vector case.
func matmul(a, b *ndarray) *ndarray {
	switch {
	case a.ndim() == 1 && b.ndim() == 1:
		if a.shape[0] != b.shape[0] {
			panic(vm.Errorf("ndarray:matmul: length mismatch %d vs %d", a.shape[0], b.shape[0]))
		}
		s := 0.0
		for i := range a.data {
			s += a.data[i] * b.data[i]
		}
		return scalarND(s)
	case a.ndim() == 2 && b.ndim() == 2:
		m, k, k2, n := a.shape[0], a.shape[1], b.shape[0], b.shape[1]
		if k != k2 {
			panic(vm.Errorf("ndarray:matmul: inner dimensions disagree (%dx%d · %dx%d)", m, k, k2, n))
		}
		r := newND([]int{m, n})
		for i := 0; i < m; i++ {
			for p := 0; p < k; p++ {
				aip := a.data[i*k+p]
				if aip == 0 {
					continue
				}
				brow := p * n
				orow := i * n
				for j := 0; j < n; j++ {
					r.data[orow+j] += aip * b.data[brow+j]
				}
			}
		}
		return r
	case a.ndim() == 2 && b.ndim() == 1:
		m, k := a.shape[0], a.shape[1]
		if k != b.shape[0] {
			panic(vm.Errorf("ndarray:matmul: %dx%d · %d shape mismatch", m, k, b.shape[0]))
		}
		r := newND([]int{m})
		for i := 0; i < m; i++ {
			s := 0.0
			for p := 0; p < k; p++ {
				s += a.data[i*k+p] * b.data[p]
			}
			r.data[i] = s
		}
		return r
	case a.ndim() == 1 && b.ndim() == 2:
		k, n := b.shape[0], b.shape[1]
		if a.shape[0] != k {
			panic(vm.Errorf("ndarray:matmul: %d · %dx%d shape mismatch", a.shape[0], k, n))
		}
		r := newND([]int{n})
		for j := 0; j < n; j++ {
			s := 0.0
			for p := 0; p < k; p++ {
				s += a.data[p] * b.data[p*n+j]
			}
			r.data[j] = s
		}
		return r
	default:
		panic(vm.Errorf("ndarray:matmul: only 1-D and 2-D operands are supported (got %d-D and %d-D)", a.ndim(), b.ndim()))
	}
}
