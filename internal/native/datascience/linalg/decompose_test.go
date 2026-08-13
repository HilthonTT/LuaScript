package linalg

import (
	"math"
	"testing"
)

// Each decomposition is checked against the identity that defines it rather
// than against a table of expected numbers: a hard-coded matrix would only
// confirm that the code still does whatever it did when the test was written.

func closeTo(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func assertMatrixEqual(t *testing.T, got, want [][]float64, eps float64, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d rows, want %d", what, len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("%s: row %d has %d cols, want %d", what, i, len(got[i]), len(want[i]))
		}
		for j := range want[i] {
			if !closeTo(got[i][j], want[i][j], eps) {
				t.Errorf("%s[%d][%d] = %g, want %g", what, i, j, got[i][j], want[i][j])
			}
		}
	}
}

func matmulRaw(a, b [][]float64) [][]float64 {
	out := zeroMatrix(len(a), cols(b))
	for i := range a {
		for j := range cols(b) {
			s := 0.0
			for k := range cols(a) {
				s += a[i][k] * b[k][j]
			}
			out[i][j] = s
		}
	}
	return out
}

func transposeRaw(a [][]float64) [][]float64 {
	out := zeroMatrix(cols(a), len(a))
	for i := range a {
		for j := range a[i] {
			out[j][i] = a[i][j]
		}
	}
	return out
}

// L*Lᵀ must reproduce A.
func TestCholeskyReconstructsInput(t *testing.T) {
	a := [][]float64{
		{4, 12, -16},
		{12, 37, -43},
		{-16, -43, 98},
	}
	l, ok := cholesky(a)
	if !ok {
		t.Fatal("cholesky failed on a positive definite matrix")
	}
	// L must be lower triangular.
	for i := range l {
		for j := i + 1; j < len(l); j++ {
			if l[i][j] != 0 {
				t.Errorf("L[%d][%d] = %g, want 0 (not lower triangular)", i, j, l[i][j])
			}
		}
	}
	assertMatrixEqual(t, matmulRaw(l, transposeRaw(l)), a, 1e-9, "L*Lt")
}

// Failure is the useful signal: it is the test for positive definiteness.
func TestCholeskyRejectsNonPositiveDefinite(t *testing.T) {
	// Symmetric but indefinite (eigenvalues of opposite sign).
	if _, ok := cholesky([][]float64{{0, 1}, {1, 0}}); ok {
		t.Error("cholesky accepted an indefinite matrix")
	}
	// Symmetric negative definite.
	if _, ok := cholesky([][]float64{{-1, 0}, {0, -1}}); ok {
		t.Error("cholesky accepted a negative definite matrix")
	}
	// Not symmetric at all.
	if _, ok := cholesky([][]float64{{1, 2}, {3, 1}}); ok {
		t.Error("cholesky accepted a non-symmetric matrix")
	}
}

// Q*R must reproduce A, and QᵀQ must be the identity.
func TestQRReconstructsAndIsOrthonormal(t *testing.T) {
	a := [][]float64{
		{12, -51, 4},
		{6, 167, -68},
		{-4, 24, -41},
	}
	q, r, ok := qrDecompose(a)
	if !ok {
		t.Fatal("qrDecompose failed on a full-rank matrix")
	}
	assertMatrixEqual(t, matmulRaw(q, r), a, 1e-9, "Q*R")
	assertMatrixEqual(t, matmulRaw(transposeRaw(q), q), identityMatrix(3), 1e-9, "Qt*Q")

	// R must be upper triangular.
	for i := range r {
		for j := range i {
			if math.Abs(r[i][j]) > 1e-12 {
				t.Errorf("R[%d][%d] = %g, want 0 (not upper triangular)", i, j, r[i][j])
			}
		}
	}
}

// A tall matrix is the shape least squares actually needs.
func TestQRHandlesTallMatrix(t *testing.T) {
	a := [][]float64{{1, 1}, {1, 2}, {1, 3}, {1, 4}}
	q, r, ok := qrDecompose(a)
	if !ok {
		t.Fatal("qrDecompose failed on a tall full-rank matrix")
	}
	assertMatrixEqual(t, matmulRaw(q, r), a, 1e-9, "Q*R")
	assertMatrixEqual(t, matmulRaw(transposeRaw(q), q), identityMatrix(2), 1e-9, "Qt*Q")
}

func TestQRRejectsDependentColumns(t *testing.T) {
	// Second column is twice the first.
	if _, _, ok := qrDecompose([][]float64{{1, 2}, {2, 4}, {3, 6}}); ok {
		t.Error("qrDecompose accepted linearly dependent columns")
	}
}

// A perfect fit must be recovered exactly: y = 2x + 1 through four points.
func TestLstsqExactFit(t *testing.T) {
	a := [][]float64{{1, 1}, {1, 2}, {1, 3}, {1, 4}}
	b := []float64{3, 5, 7, 9}
	x, ok := lstsq(a, b)
	if !ok {
		t.Fatal("lstsq failed")
	}
	if !closeTo(x[0], 1, 1e-9) || !closeTo(x[1], 2, 1e-9) {
		t.Errorf("lstsq = [%g, %g], want [1, 2] (intercept 1, slope 2)", x[0], x[1])
	}
}

// With noise there is no exact solution, so check the defining property
// instead: the residual must be orthogonal to every column of A.
func TestLstsqResidualIsOrthogonalToColumns(t *testing.T) {
	a := [][]float64{{1, 1}, {1, 2}, {1, 3}, {1, 4}}
	b := []float64{3.1, 4.9, 7.2, 8.8}
	x, ok := lstsq(a, b)
	if !ok {
		t.Fatal("lstsq failed")
	}
	resid := make([]float64, len(b))
	for i := range b {
		pred := 0.0
		for j := range cols(a) {
			pred += a[i][j] * x[j]
		}
		resid[i] = b[i] - pred
	}
	for j := range cols(a) {
		d := 0.0
		for i := range a {
			d += a[i][j] * resid[i]
		}
		if math.Abs(d) > 1e-9 {
			t.Errorf("residual is not orthogonal to column %d (dot = %g); the fit is not least squares", j, d)
		}
	}
}

func TestRank(t *testing.T) {
	cases := []struct {
		name string
		m    [][]float64
		want int
	}{
		{"identity", identityMatrix(3), 3},
		{"zero", zeroMatrix(3, 3), 0},
		{"duplicate row", [][]float64{{1, 2}, {2, 4}}, 1},
		{"full rank 2x2", [][]float64{{1, 2}, {3, 4}}, 2},
		{"third row is the sum of the first two", [][]float64{{1, 0, 0}, {0, 1, 0}, {1, 1, 0}}, 2},
		{"wide", [][]float64{{1, 2, 3}, {4, 5, 6}}, 2},
	}
	for _, c := range cases {
		if got := rank(c.m); got != c.want {
			t.Errorf("%s: rank = %d, want %d", c.name, got, c.want)
		}
	}
}

// A diagonal matrix has its diagonal as eigenvalues, which pins both the
// values and the descending order.
func TestEighDiagonal(t *testing.T) {
	a := [][]float64{{3, 0, 0}, {0, 1, 0}, {0, 0, 2}}
	values, _ := eigenSymmetric(a)
	want := []float64{3, 2, 1} // descending
	for i := range want {
		if !closeTo(values[i], want[i], 1e-9) {
			t.Errorf("eigenvalue %d = %g, want %g", i, values[i], want[i])
		}
	}
}

// The defining property: A*v = lambda*v for each eigenpair.
func TestEighSatisfiesEigenEquation(t *testing.T) {
	a := [][]float64{
		{6, -2, -1},
		{-2, 6, -1},
		{-1, -1, 5},
	}
	values, vectors := eigenSymmetric(a)
	n := len(a)
	for k := range n {
		for i := range n {
			av := 0.0
			for j := range n {
				av += a[i][j] * vectors[j][k]
			}
			if !closeTo(av, values[k]*vectors[i][k], 1e-8) {
				t.Errorf("eigenpair %d, row %d: A*v = %g but lambda*v = %g",
					k, i, av, values[k]*vectors[i][k])
			}
		}
	}
}

// Eigenvectors of a symmetric matrix are orthonormal, and the eigenvalues must
// sum to the trace.
func TestEighVectorsOrthonormalAndTracePreserved(t *testing.T) {
	a := [][]float64{
		{4, 1, 0},
		{1, 3, 1},
		{0, 1, 2},
	}
	values, vectors := eigenSymmetric(a)
	assertMatrixEqual(t, matmulRaw(transposeRaw(vectors), vectors), identityMatrix(3), 1e-8, "Vt*V")

	trace, sum := 0.0, 0.0
	for i := range a {
		trace += a[i][i]
		sum += values[i]
	}
	if !closeTo(trace, sum, 1e-8) {
		t.Errorf("eigenvalues sum to %g, want the trace %g", sum, trace)
	}
}

func TestIsSymmetric(t *testing.T) {
	if !isSymmetric([][]float64{{1, 2}, {2, 3}}) {
		t.Error("symmetric matrix reported as asymmetric")
	}
	if isSymmetric([][]float64{{1, 2}, {3, 4}}) {
		t.Error("asymmetric matrix reported as symmetric")
	}
	// Tolerant of the rounding a covariance matrix accumulates.
	if !isSymmetric([][]float64{{1, 2}, {2 + 1e-12, 3}}) {
		t.Error("rounding-level asymmetry should still count as symmetric")
	}
	if isSymmetric([][]float64{{1, 2, 3}, {2, 3, 4}}) {
		t.Error("non-square matrix reported as symmetric")
	}
}
