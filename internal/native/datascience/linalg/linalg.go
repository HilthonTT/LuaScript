// Package linalg is a require()-able host module providing the vector and
// matrix primitives that underpin most numeric data-science work: dot
// products, norms, matrix multiplication, transpose, and the small dense
// solvers (determinant, inverse, linear solve via Gaussian elimination with
// partial pivoting) needed for, e.g., closed-form linear regression.
//
// Conventions: a vector is a Lua array of numbers; a matrix is a Lua array
// of equal-length row arrays. The numeric core operates on []float64 and
// [][]float64 and is tested directly in linalg_test.go.
package linalg

import (
	"math"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterLinalgPreload installs the loader under package.preload.
func RegisterLinalgPreload(v *vm.VM) {
	vm.RegisterPreload(v, "linalg", linalgLoader)
}

func linalgLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 4)
	methods := vm.NewTable(0, 24)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "linalg:" + name, Fn: fn})
	}

	// --- vector ops -------------------------------------------------------
	set("dot", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vec("linalg.dot", 1, args)
		b := vec("linalg.dot", 2, args)
		requireSameLen("linalg.dot", a, b)
		return []vm.Value{dot(a, b)}
	})
	set("norm", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{norm(vec("linalg.norm", 1, args))}
	})
	set("add", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vec("linalg.add", 1, args)
		b := vec("linalg.add", 2, args)
		requireSameLen("linalg.add", a, b)
		return []vm.Value{vecToTable(zipWith(a, b, func(x, y float64) float64 { return x + y }))}
	})
	set("sub", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vec("linalg.sub", 1, args)
		b := vec("linalg.sub", 2, args)
		requireSameLen("linalg.sub", a, b)
		return []vm.Value{vecToTable(zipWith(a, b, func(x, y float64) float64 { return x - y }))}
	})
	set("scale", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vec("linalg.scale", 1, args)
		s := vm.FloatArg("linalg.scale", 2, args)
		out := make([]float64, len(a))
		for i, x := range a {
			out[i] = x * s
		}
		return []vm.Value{vecToTable(out)}
	})
	set("distance", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := vec("linalg.distance", 1, args)
		b := vec("linalg.distance", 2, args)
		requireSameLen("linalg.distance", a, b)
		return []vm.Value{norm(zipWith(a, b, func(x, y float64) float64 { return x - y }))}
	})

	// --- matrix ops -------------------------------------------------------
	set("matmul", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := mat("linalg.matmul", 1, args)
		b := mat("linalg.matmul", 2, args)
		c, err := matmul(a, b)
		if err != nil {
			panic(vm.Errorf("linalg.matmul: %s", err.Error()))
		}
		return []vm.Value{matToTable(c)}
	})
	set("matvec", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := mat("linalg.matvec", 1, args)
		x := vec("linalg.matvec", 2, args)
		if cols(a) != len(x) {
			panic(vm.Errorf("linalg.matvec: matrix has %d columns but vector has length %d", cols(a), len(x)))
		}
		return []vm.Value{vecToTable(matvec(a, x))}
	})
	set("transpose", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{matToTable(transpose(mat("linalg.transpose", 1, args)))}
	})
	set("identity", func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := int(vm.IntArg("linalg.identity", 1, args))
		if n < 1 {
			panic(vm.Errorf("linalg.identity: size must be >= 1, got %d", n))
		}
		checkDims("linalg.identity", n, n)
		return []vm.Value{matToTable(identity(n))}
	})
	set("zeros", func(_ *vm.VM, args []vm.Value) []vm.Value {
		r := int(vm.IntArg("linalg.zeros", 1, args))
		c := int(vm.IntArg("linalg.zeros", 2, args))
		checkDims("linalg.zeros", r, c)
		return []vm.Value{matToTable(filled(r, c, 0))}
	})
	set("ones", func(_ *vm.VM, args []vm.Value) []vm.Value {
		r := int(vm.IntArg("linalg.ones", 1, args))
		c := int(vm.IntArg("linalg.ones", 2, args))
		checkDims("linalg.ones", r, c)
		return []vm.Value{matToTable(filled(r, c, 1))}
	})
	set("trace", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := mat("linalg.trace", 1, args)
		requireSquare("linalg.trace", a)
		var s float64
		for i := range a {
			s += a[i][i]
		}
		return []vm.Value{s}
	})
	set("det", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := mat("linalg.det", 1, args)
		requireSquare("linalg.det", a)
		return []vm.Value{determinant(a)}
	})
	set("inverse", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := mat("linalg.inverse", 1, args)
		requireSquare("linalg.inverse", a)
		inv, ok := inverse(a)
		if !ok {
			panic(vm.Errorf("linalg.inverse: matrix is singular"))
		}
		return []vm.Value{matToTable(inv)}
	})
	// solve(A, b) -> x such that A x = b.
	set("solve", func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := mat("linalg.solve", 1, args)
		b := vec("linalg.solve", 2, args)
		requireSquare("linalg.solve", a)
		if len(a) != len(b) {
			panic(vm.Errorf("linalg.solve: A is %dx%d but b has length %d", len(a), cols(a), len(b)))
		}
		x, ok := solve(a, b)
		if !ok {
			panic(vm.Errorf("linalg.solve: matrix is singular"))
		}
		return []vm.Value{vecToTable(x)}
	})

	m.Set("VERSION", "0.1.0")
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return []vm.Value{m}
}

// ---------------------------------------------------------------------------
// Numeric core
// ---------------------------------------------------------------------------

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func norm(a []float64) float64 { return math.Sqrt(dot(a, a)) }

func zipWith(a, b []float64, fn func(x, y float64) float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		out[i] = fn(a[i], b[i])
	}
	return out
}

func cols(a [][]float64) int {
	if len(a) == 0 {
		return 0
	}
	return len(a[0])
}

func matmul(a, b [][]float64) ([][]float64, error) {
	if cols(a) != len(b) {
		return nil, errShape(len(a), cols(a), len(b), cols(b))
	}
	n, m, p := len(a), cols(a), cols(b)
	out := filled(n, p, 0)
	for i := range n {
		for k := range m {
			aik := a[i][k]
			for j := 0; j < p; j++ {
				out[i][j] += aik * b[k][j]
			}
		}
	}
	return out, nil
}

func matvec(a [][]float64, x []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		out[i] = dot(a[i], x)
	}
	return out
}

func transpose(a [][]float64) [][]float64 {
	out := filled(cols(a), len(a), 0)
	for i := range a {
		for j := range a[i] {
			out[j][i] = a[i][j]
		}
	}
	return out
}

func identity(n int) [][]float64 {
	out := filled(n, n, 0)
	for i := 0; i < n; i++ {
		out[i][i] = 1
	}
	return out
}

// maxMatElems bounds script-supplied matrix dimensions so they can't force
// an unrecoverable OOM (fatal, not pcall-catchable) via zeros/ones/identity.
const maxMatElems = 1 << 26 // 64M elements ≈ 512 MiB of float64

func checkDims(fn string, r, c int) {
	if r < 0 || c < 0 {
		panic(vm.Errorf("%s: dimensions must be non-negative, got %dx%d", fn, r, c))
	}
	if r > 0 && c > maxMatElems/r {
		panic(vm.Errorf("%s: %dx%d exceeds %d elements", fn, r, c, maxMatElems))
	}
}

func filled(r, c int, v float64) [][]float64 {
	out := make([][]float64, r)
	for i := range out {
		out[i] = make([]float64, c)
		if v != 0 {
			for j := range out[i] {
				out[i][j] = v
			}
		}
	}
	return out
}

// determinant via LU decomposition with partial pivoting. Returns 0 for a
// singular matrix (a zero pivot).
func determinant(a [][]float64) float64 {
	lu, _, sign, ok := luDecompose(a)
	if !ok {
		return 0
	}
	det := float64(sign)
	for i := range lu {
		det *= lu[i][i]
	}
	return det
}

// inverse returns A⁻¹ by solving A X = I column by column. ok is false when A
// is singular.
func inverse(a [][]float64) ([][]float64, bool) {
	n := len(a)
	lu, piv, _, ok := luDecompose(a)
	if !ok {
		return nil, false
	}
	inv := filled(n, n, 0)
	for c := range n {
		e := make([]float64, n)
		e[c] = 1
		x := luSolve(lu, piv, e)
		for r := 0; r < n; r++ {
			inv[r][c] = x[r]
		}
	}
	return inv, true
}

// solve returns x with A x = b. ok is false when A is singular.
func solve(a [][]float64, b []float64) ([]float64, bool) {
	lu, piv, _, ok := luDecompose(a)
	if !ok {
		return nil, false
	}
	return luSolve(lu, piv, b), true
}

// luDecompose performs LU decomposition with partial pivoting in place on a
// copy of a. It returns the combined L/U matrix, the pivot permutation, the
// permutation sign (for the determinant), and ok=false if a zero pivot makes
// the matrix singular.
func luDecompose(a [][]float64) (lu [][]float64, piv []int, sign int, ok bool) {
	n := len(a)
	lu = filled(n, n, 0)
	for i := range a {
		copy(lu[i], a[i])
	}
	piv = make([]int, n)
	for i := range piv {
		piv[i] = i
	}
	sign = 1

	for col := 0; col < n; col++ {
		// Partial pivot: pick the largest-magnitude entry in this column.
		p, max := col, math.Abs(lu[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(lu[r][col]); v > max {
				p, max = r, v
			}
		}
		if max == 0 {
			return nil, nil, 0, false
		}
		if p != col {
			lu[p], lu[col] = lu[col], lu[p]
			piv[p], piv[col] = piv[col], piv[p]
			sign = -sign
		}
		for r := col + 1; r < n; r++ {
			lu[r][col] /= lu[col][col]
			f := lu[r][col]
			for c := col + 1; c < n; c++ {
				lu[r][c] -= f * lu[col][c]
			}
		}
	}
	return lu, piv, sign, true
}

// luSolve solves L U x = P b given a decomposition from luDecompose.
func luSolve(lu [][]float64, piv []int, b []float64) []float64 {
	n := len(lu)
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = b[piv[i]]
	}
	// Forward substitution (L has unit diagonal).
	for i := range n {
		for j := 0; j < i; j++ {
			x[i] -= lu[i][j] * x[j]
		}
	}
	// Back substitution.
	for i := n - 1; i >= 0; i-- {
		for j := i + 1; j < n; j++ {
			x[i] -= lu[i][j] * x[j]
		}
		x[i] /= lu[i][i]
	}
	return x
}

// ---------------------------------------------------------------------------
// Marshalling helpers
// ---------------------------------------------------------------------------

func vec(site string, n int, args []vm.Value) []float64 {
	t := vm.TableArg(site, n, args)
	length := int(t.Len())
	if length == 0 {
		panic(vm.Errorf("%s: argument #%d must be a non-empty vector", site, n))
	}
	out := make([]float64, length)
	for i := 1; i <= length; i++ {
		f, ok := vm.ToFloat(t.Get(int64(i)))
		if !ok {
			panic(vm.Errorf("%s: vector element %d is not a number", site, i))
		}
		out[i-1] = f
	}
	return out
}

// mat reads a matrix and enforces that every row shares the first row's width.
func mat(site string, n int, args []vm.Value) [][]float64 {
	t := vm.TableArg(site, n, args)
	rows := int(t.Len())
	if rows == 0 {
		panic(vm.Errorf("%s: argument #%d must be a non-empty matrix", site, n))
	}
	out := make([][]float64, rows)
	width := -1
	for i := 1; i <= rows; i++ {
		row, ok := t.Get(int64(i)).(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: row %d must be an array of numbers", site, i))
		}
		w := int(row.Len())
		if width == -1 {
			width = w
		} else if w != width {
			panic(vm.Errorf("%s: row %d has width %d, expected %d (matrix must be rectangular)", site, i, w, width))
		}
		r := make([]float64, w)
		for j := 1; j <= w; j++ {
			f, ok := vm.ToFloat(row.Get(int64(j)))
			if !ok {
				panic(vm.Errorf("%s: element [%d][%d] is not a number", site, i, j))
			}
			r[j-1] = f
		}
		out[i-1] = r
	}
	return out
}

func requireSameLen(site string, a, b []float64) {
	if len(a) != len(b) {
		panic(vm.Errorf("%s: vectors must be the same length (%d vs %d)", site, len(a), len(b)))
	}
}

func requireSquare(site string, a [][]float64) {
	if len(a) != cols(a) {
		panic(vm.Errorf("%s: matrix must be square (%dx%d)", site, len(a), cols(a)))
	}
}

func errShape(ar, ac, br, bc int) error {
	return vm.Errorf("incompatible shapes %dx%d and %dx%d", ar, ac, br, bc)
}

func vecToTable(xs []float64) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	for i, v := range xs {
		t.Set(int64(i+1), v)
	}
	return t
}

func matToTable(a [][]float64) *vm.Table {
	t := vm.NewTable(len(a), 0)
	for i, row := range a {
		t.Set(int64(i+1), vecToTable(row))
	}
	return t
}
