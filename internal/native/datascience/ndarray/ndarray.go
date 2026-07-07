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
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

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

// ---------------------------------------------------------------------------
// Core numeric layer (VM-independent; exercised directly in the tests)
// ---------------------------------------------------------------------------

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

func shapeStr(shape []int) string {
	parts := make([]string, len(shape))
	for i, d := range shape {
		parts[i] = strconv.Itoa(d)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// ---------------------------------------------------------------------------
// Lua marshalling
// ---------------------------------------------------------------------------

// ndKey is the (private) instance-table key under which the backing *ndarray
// is stored. Prefixed with a control byte to avoid colliding with any field a
// user would reasonably index.
const ndKey = "\x00ndarray"

var (
	ndMeta     *vm.Table
	metaOnce   sync.Once
	ndMethods  *vm.Table // shared __index method table
)

// wrap exposes an *ndarray as a Lua object sharing a single metatable; the
// backing pointer rides on the instance table under ndKey.
func wrap(a *ndarray) *vm.Table {
	metaOnce.Do(buildMeta)
	t := vm.NewTable(0, 1)
	t.Set(ndKey, a)
	t.SetMetatable(ndMeta)
	return t
}

// asND coerces a Lua value to an *ndarray: numbers become scalars, wrapped
// arrays unwrap. ok is false for anything else.
func asND(v vm.Value) (*ndarray, bool) {
	switch x := v.(type) {
	case int64:
		return scalarND(float64(x)), true
	case float64:
		return scalarND(x), true
	case *vm.Table:
		if p, ok := x.Get(ndKey).(*ndarray); ok {
			return p, true
		}
	}
	return nil, false
}

func ndArg(site string, v vm.Value) *ndarray {
	if a, ok := asND(v); ok {
		return a
	}
	panic(vm.Errorf("%s: expected an ndarray or number, got %s", site, vm.TypeName(v)))
}

// selfND recovers the receiver of a colon method call (args[0]).
func selfND(site string, args []vm.Value) *ndarray {
	if len(args) == 0 {
		panic(vm.Errorf("%s: called without a receiver (use a:method(), not a.method())", site))
	}
	t, ok := args[0].(*vm.Table)
	if !ok {
		panic(vm.Errorf("%s: receiver is not an ndarray", site))
	}
	p, ok := t.Get(ndKey).(*ndarray)
	if !ok {
		panic(vm.Errorf("%s: receiver is not an ndarray", site))
	}
	return p
}

func argAt(args []vm.Value, i int) vm.Value {
	if i < 1 || i > len(args) {
		return nil
	}
	return args[i-1]
}

// ---------------------------------------------------------------------------
// Module loader
// ---------------------------------------------------------------------------

func ndLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 16)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		m.Set(name, &vm.GoFunc{Name: "ndarray." + name, Fn: fn})
	}

	set("array", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(fromNested("ndarray.array", vm.TableArg("ndarray.array", 1, args)))}
	})
	set("zeros", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(newND(shapeArgs("ndarray.zeros", args, 1)))}
	})
	set("ones", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := newND(shapeArgs("ndarray.ones", args, 1))
		for i := range a.data {
			a.data[i] = 1
		}
		return []vm.Value{wrap(a)}
	})
	set("full", func(_ *vm.VM, args []vm.Value) []vm.Value {
		// full(value, dims...) — value first so dims stay variadic.
		val := vm.FloatArg("ndarray.full", 1, args)
		a := newND(shapeArgs("ndarray.full", args, 2))
		for i := range a.data {
			a.data[i] = val
		}
		return []vm.Value{wrap(a)}
	})
	set("arange", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(arange(args))}
	})
	set("linspace", func(_ *vm.VM, args []vm.Value) []vm.Value {
		lo := vm.FloatArg("ndarray.linspace", 1, args)
		hi := vm.FloatArg("ndarray.linspace", 2, args)
		n := int(vm.IntArg("ndarray.linspace", 3, args))
		if n < 0 {
			panic(vm.Errorf("ndarray.linspace: count must be >= 0"))
		}
		a := newND([]int{n})
		if n == 1 {
			a.data[0] = lo
		} else {
			step := (hi - lo) / float64(n-1)
			for i := 0; i < n; i++ {
				a.data[i] = lo + step*float64(i)
			}
		}
		return []vm.Value{wrap(a)}
	})
	set("eye", func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := int(vm.IntArg("ndarray.eye", 1, args))
		a := newND([]int{n, n})
		for i := 0; i < n; i++ {
			a.data[i*n+i] = 1
		}
		return []vm.Value{wrap(a)}
	})
	set("from_table", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(fromNested("ndarray.from_table", vm.TableArg("ndarray.from_table", 1, args)))}
	})
	set("matmul", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := ndArg("ndarray.matmul", argAt(args, 1))
		b := ndArg("ndarray.matmul", argAt(args, 2))
		return []vm.Value{result(matmul(a, b))}
	})
	set("concat", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := ndArg("ndarray.concat", argAt(args, 1))
		b := ndArg("ndarray.concat", argAt(args, 2))
		return []vm.Value{wrap(concat1D("ndarray.concat", a, b))}
	})
	set("is_ndarray", func(_ *vm.VM, args []vm.Value) []vm.Value {
		if t, ok := argAt(args, 1).(*vm.Table); ok {
			if _, ok := t.Get(ndKey).(*ndarray); ok {
				return []vm.Value{true}
			}
		}
		return []vm.Value{false}
	})

	m.Set("VERSION", "0.1.0")
	return []vm.Value{m}
}

// shapeArgs reads dimensions from args starting at position `from`. A single
// table argument ({2, 3}) is accepted as well as varargs (2, 3).
func shapeArgs(site string, args []vm.Value, from int) []int {
	if t, ok := argAt(args, from).(*vm.Table); ok {
		n := int(t.Len())
		shape := make([]int, n)
		for i := 1; i <= n; i++ {
			shape[i-1] = int(vm.IntArg(site, 1, []vm.Value{t.Get(int64(i))}))
		}
		return shape
	}
	shape := []int{}
	for i := from; i <= len(args); i++ {
		shape = append(shape, int(vm.IntArg(site, i, args)))
	}
	if len(shape) == 0 {
		panic(vm.Errorf("%s: expected at least one dimension", site))
	}
	return shape
}

func arange(args []vm.Value) *ndarray {
	// arange(stop) | arange(start, stop) | arange(start, stop, step)
	var start, stop, step float64 = 0, 0, 1
	switch len(args) {
	case 1:
		stop = vm.FloatArg("ndarray.arange", 1, args)
	case 2:
		start = vm.FloatArg("ndarray.arange", 1, args)
		stop = vm.FloatArg("ndarray.arange", 2, args)
	default:
		start = vm.FloatArg("ndarray.arange", 1, args)
		stop = vm.FloatArg("ndarray.arange", 2, args)
		step = vm.FloatArg("ndarray.arange", 3, args)
	}
	if step == 0 {
		panic(vm.Errorf("ndarray.arange: step must be non-zero"))
	}
	var vals []float64
	if step > 0 {
		for x := start; x < stop; x += step {
			vals = append(vals, x)
		}
	} else {
		for x := start; x > stop; x += step {
			vals = append(vals, x)
		}
	}
	a := newND([]int{len(vals)})
	copy(a.data, vals)
	return a
}

func concat1D(site string, a, b *ndarray) *ndarray {
	if a.ndim() != 1 || b.ndim() != 1 {
		panic(vm.Errorf("%s: concat currently supports 1-D arrays only", site))
	}
	r := newND([]int{a.size() + b.size()})
	copy(r.data, a.data)
	copy(r.data[a.size():], b.data)
	return r
}

// fromNested builds an array from nested Lua arrays, inferring the shape from
// the first element at each depth and validating that the structure is
// rectangular.
func fromNested(site string, t *vm.Table) *ndarray {
	shape := inferShape(t)
	a := newND(shape)
	idx := 0
	var fill func(v vm.Value, depth int)
	fill = func(v vm.Value, depth int) {
		if depth == len(shape) {
			f, ok := vm.ToFloat(v)
			if !ok {
				panic(vm.Errorf("%s: non-numeric element %s", site, vm.TypeName(v)))
			}
			a.data[idx] = f
			idx++
			return
		}
		tv, ok := v.(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: ragged nesting — expected a sub-array at depth %d", site, depth))
		}
		if int(tv.Len()) != shape[depth] {
			panic(vm.Errorf("%s: ragged array — axis %d has inconsistent length", site, depth))
		}
		for i := 1; i <= shape[depth]; i++ {
			fill(tv.Get(int64(i)), depth+1)
		}
	}
	fill(t, 0)
	return a
}

func inferShape(t *vm.Table) []int {
	shape := []int{}
	var v vm.Value = t
	for {
		tv, ok := v.(*vm.Table)
		if !ok {
			break
		}
		n := int(tv.Len())
		shape = append(shape, n)
		if n == 0 {
			break
		}
		v = tv.Get(int64(1))
	}
	return shape
}

// result returns a 0-D array as a bare Lua number, and any higher-rank array
// wrapped. This keeps scalar reductions/dot products ergonomic in Lua.
func result(a *ndarray) vm.Value {
	if a.ndim() == 0 {
		return a.data[0]
	}
	return wrap(a)
}

// ---------------------------------------------------------------------------
// Metatable (shared across all instances)
// ---------------------------------------------------------------------------

func buildMeta() {
	ndMethods = vm.NewTable(0, 32)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		ndMethods.Set(name, &vm.GoFunc{Name: "ndarray:" + name, Fn: fn})
	}

	// --- shape / introspection -------------------------------------------
	set("shape", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:shape", args)
		t := vm.NewTable(len(a.shape), 0)
		for i, d := range a.shape {
			t.Set(int64(i+1), int64(d))
		}
		return []vm.Value{t}
	})
	set("ndim", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{int64(selfND("ndarray:ndim", args).ndim())}
	})
	set("size", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{int64(selfND("ndarray:size", args).size())}
	})

	// --- element access ---------------------------------------------------
	set("get", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:get", args)
		return []vm.Value{a.data[a.flatIndex("ndarray:get", args[1:])]}
	})
	set("set", func(_ *vm.VM, args []vm.Value) []vm.Value {
		// set(value, i, j, ...) — value first, then the (1-based) indices.
		a := selfND("ndarray:set", args)
		val := vm.FloatArg("ndarray:set", 2, args)
		a.data[a.flatIndex("ndarray:set", args[2:])] = val
		return nil
	})

	// --- structural transforms -------------------------------------------
	set("reshape", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:reshape", args)
		return []vm.Value{wrap(a.reshape("ndarray:reshape", shapeArgs("ndarray:reshape", args, 2)))}
	})
	set("flatten", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:flatten", args)
		return []vm.Value{wrap(a.reshape("ndarray:flatten", []int{a.size()}))}
	})
	set("transpose", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:transpose", args)
		var perm []int
		if len(args) > 1 {
			perm = shapeArgs("ndarray:transpose", args, 2)
		}
		return []vm.Value{wrap(a.transpose("ndarray:transpose", perm))}
	})
	set("copy", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:copy", args)
		return []vm.Value{wrap(&ndarray{data: append([]float64(nil), a.data...), shape: append([]int(nil), a.shape...)})}
	})

	// --- reductions -------------------------------------------------------
	reduction := func(name string, all func([]float64) float64) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := selfND("ndarray:"+name, args)
			if ax := argAt(args, 2); ax != nil {
				axis := int(vm.IntArg("ndarray:"+name, 2, args)) - 1
				return []vm.Value{result(a.alongAxis("ndarray:"+name, axis, all))}
			}
			return []vm.Value{a.reduceAll(all)}
		})
	}
	reduction("sum", sumf)
	reduction("mean", meanf)
	reduction("prod", prodf)
	reduction("max", maxf)
	reduction("min", minf)
	reduction("std", stdf)
	reduction("var", varf)

	set("argmax", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:argmax", args)
		return []vm.Value{int64(argExtreme(a.data, true) + 1)}
	})
	set("argmin", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:argmin", args)
		return []vm.Value{int64(argExtreme(a.data, false) + 1)}
	})

	// --- elementwise math -------------------------------------------------
	unary := func(name string, f func(float64) float64) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			return []vm.Value{wrap(selfND("ndarray:"+name, args).unary(f))}
		})
	}
	unary("abs", math.Abs)
	unary("exp", math.Exp)
	unary("log", math.Log)
	unary("sqrt", math.Sqrt)
	unary("sin", math.Sin)
	unary("cos", math.Cos)
	unary("tanh", math.Tanh)
	unary("floor", math.Floor)
	unary("ceil", math.Ceil)
	unary("neg", func(x float64) float64 { return -x })

	set("pow", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:pow", args)
		e := vm.FloatArg("ndarray:pow", 2, args)
		return []vm.Value{wrap(a.unary(func(x float64) float64 { return math.Pow(x, e) }))}
	})
	set("clip", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:clip", args)
		lo := vm.FloatArg("ndarray:clip", 2, args)
		hi := vm.FloatArg("ndarray:clip", 3, args)
		return []vm.Value{wrap(a.unary(func(x float64) float64 {
			switch {
			case x < lo:
				return lo
			case x > hi:
				return hi
			default:
				return x
			}
		}))}
	})

	// --- binary / linear algebra -----------------------------------------
	binary := func(name string, op func(x, y float64) float64) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := selfND("ndarray:"+name, args)
			b := ndArg("ndarray:"+name, argAt(args, 2))
			return []vm.Value{wrap(ewise("ndarray:"+name, a, b, op))}
		})
	}
	binary("add", func(x, y float64) float64 { return x + y })
	binary("sub", func(x, y float64) float64 { return x - y })
	binary("mul", func(x, y float64) float64 { return x * y })
	binary("div", func(x, y float64) float64 { return x / y })

	set("dot", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:dot", args)
		b := ndArg("ndarray:dot", argAt(args, 2))
		return []vm.Value{result(matmul(a, b))}
	})
	set("matmul", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:matmul", args)
		b := ndArg("ndarray:matmul", argAt(args, 2))
		return []vm.Value{result(matmul(a, b))}
	})

	// --- higher order & conversion ---------------------------------------
	set("map", func(v *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:map", args)
		fn := argAt(args, 2)
		switch fn.(type) {
		case *vm.Closure, *vm.GoFunc:
		default:
			panic(vm.Errorf("ndarray:map: argument must be a function, got %s", vm.TypeName(fn)))
		}
		r := &ndarray{data: make([]float64, len(a.data)), shape: append([]int(nil), a.shape...)}
		for i, x := range a.data {
			res := v.CallValue(fn, []vm.Value{x, int64(i + 1)}, 1)
			if len(res) > 0 {
				f, ok := vm.ToFloat(res[0])
				if !ok {
					panic(vm.Errorf("ndarray:map: function must return a number, got %s", vm.TypeName(res[0])))
				}
				r.data[i] = f
			}
		}
		return []vm.Value{wrap(r)}
	})
	set("to_table", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{selfND("ndarray:to_table", args).toTable()}
	})
	set("tolist", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:tolist", args)
		t := vm.NewTable(len(a.data), 0)
		for i, x := range a.data {
			t.Set(int64(i+1), x)
		}
		return []vm.Value{t}
	})
	set("show", func(_ *vm.VM, args []vm.Value) []vm.Value {
		fmt.Println(selfND("ndarray:show", args).render())
		return nil
	})

	ndMeta = vm.NewTable(0, 16)
	ndMeta.Set("__index", ndMethods)
	ndMeta.Set("__tostring", &vm.GoFunc{Name: "ndarray:__tostring", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{selfND("ndarray:__tostring", args).render()}
	}})
	ndMeta.Set("__len", &vm.GoFunc{Name: "ndarray:__len", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := selfND("ndarray:__len", args)
		if a.ndim() == 0 {
			return []vm.Value{int64(1)}
		}
		return []vm.Value{int64(a.shape[0])}
	}})
	ndMeta.Set("__eq", &vm.GoFunc{Name: "ndarray:__eq", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := ndArg("ndarray:__eq", argAt(args, 1))
		b := ndArg("ndarray:__eq", argAt(args, 2))
		return []vm.Value{a.equal(b)}
	}})
	op := func(event, site string, f func(x, y float64) float64) {
		ndMeta.Set(event, &vm.GoFunc{Name: "ndarray:" + event, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := ndArg(site, argAt(args, 1))
			b := ndArg(site, argAt(args, 2))
			return []vm.Value{wrap(ewise(site, a, b, f))}
		}})
	}
	op("__add", "ndarray:+", func(x, y float64) float64 { return x + y })
	op("__sub", "ndarray:-", func(x, y float64) float64 { return x - y })
	op("__mul", "ndarray:*", func(x, y float64) float64 { return x * y })
	op("__div", "ndarray:/", func(x, y float64) float64 { return x / y })
	op("__mod", "ndarray:%", math.Mod)
	op("__pow", "ndarray:^", math.Pow)
	ndMeta.Set("__unm", &vm.GoFunc{Name: "ndarray:__unm", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{wrap(ndArg("ndarray:__unm", argAt(args, 1)).unary(func(x float64) float64 { return -x }))}
	}})
}

func argExtreme(xs []float64, wantMax bool) int {
	if len(xs) == 0 {
		panic(vm.Errorf("ndarray: arg reduction of an empty array"))
	}
	best := 0
	for i, x := range xs {
		if (wantMax && x > xs[best]) || (!wantMax && x < xs[best]) {
			best = i
		}
	}
	return best
}

// flatIndex converts 1-based per-axis Lua indices to a flat offset.
func (a *ndarray) flatIndex(site string, idxArgs []vm.Value) int {
	if len(idxArgs) != len(a.shape) {
		panic(vm.Errorf("%s: expected %d indices for a %d-D array, got %d",
			site, len(a.shape), len(a.shape), len(idxArgs)))
	}
	strides := rowStrides(a.shape)
	off := 0
	for i, v := range idxArgs {
		ix, ok := vm.ToInteger(v)
		if !ok {
			panic(vm.Errorf("%s: index %d is not an integer", site, i+1))
		}
		k := int(ix)
		if k < 1 || k > a.shape[i] {
			panic(vm.Errorf("%s: index %d out of range [1, %d] on axis %d", site, k, a.shape[i], i+1))
		}
		off += (k - 1) * strides[i]
	}
	return off
}

func (a *ndarray) equal(b *ndarray) bool {
	if len(a.shape) != len(b.shape) {
		return false
	}
	for i := range a.shape {
		if a.shape[i] != b.shape[i] {
			return false
		}
	}
	for i := range a.data {
		if a.data[i] != b.data[i] {
			return false
		}
	}
	return true
}

// toTable materializes the array as nested Lua tables.
func (a *ndarray) toTable() vm.Value {
	if a.ndim() == 0 {
		return a.data[0]
	}
	var build func(shape []int, data []float64) *vm.Table
	build = func(shape []int, data []float64) *vm.Table {
		t := vm.NewTable(shape[0], 0)
		if len(shape) == 1 {
			for i := 0; i < shape[0]; i++ {
				t.Set(int64(i+1), data[i])
			}
			return t
		}
		stride := len(data) / shape[0]
		for i := 0; i < shape[0]; i++ {
			t.Set(int64(i+1), build(shape[1:], data[i*stride:(i+1)*stride]))
		}
		return t
	}
	return build(a.shape, a.data)
}

// ---------------------------------------------------------------------------
// Rendering (NumPy-flavored, with edge truncation for large arrays)
// ---------------------------------------------------------------------------

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
