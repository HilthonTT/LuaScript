package linalg

import (
	"math"
	"testing"
)

func eqf(t *testing.T, got, want, tol float64, name string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %g, want %g", name, got, want)
	}
}

func eqMat(t *testing.T, got, want [][]float64, tol float64, name string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: row count %d != %d", name, len(got), len(want))
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("%s: row %d width mismatch", name, i)
		}
		for j := range got[i] {
			if math.Abs(got[i][j]-want[i][j]) > tol {
				t.Fatalf("%s: [%d][%d] got %g want %g", name, i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestVectorOps(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}
	eqf(t, dot(a, b), 32, 1e-9, "dot")
	eqf(t, norm([]float64{3, 4}), 5, 1e-9, "norm")
}

func TestMatmul(t *testing.T) {
	a := [][]float64{{1, 2}, {3, 4}}
	b := [][]float64{{5, 6}, {7, 8}}
	c, err := matmul(a, b)
	if err != nil {
		t.Fatal(err)
	}
	eqMat(t, c, [][]float64{{19, 22}, {43, 50}}, 1e-9, "matmul")

	if _, err := matmul([][]float64{{1, 2, 3}}, [][]float64{{1, 2}}); err == nil {
		t.Fatal("expected shape error for incompatible matmul")
	}
}

func TestTransposeIdentity(t *testing.T) {
	a := [][]float64{{1, 2, 3}, {4, 5, 6}}
	eqMat(t, transpose(a), [][]float64{{1, 4}, {2, 5}, {3, 6}}, 1e-9, "transpose")
	eqMat(t, identity(2), [][]float64{{1, 0}, {0, 1}}, 1e-9, "identity")
}

func TestDeterminant(t *testing.T) {
	eqf(t, determinant([][]float64{{1, 2}, {3, 4}}), -2, 1e-9, "det 2x2")
	eqf(t, determinant([][]float64{{6, 1, 1}, {4, -2, 5}, {2, 8, 7}}), -306, 1e-9, "det 3x3")
	eqf(t, determinant([][]float64{{1, 2}, {2, 4}}), 0, 1e-9, "singular det")
}

func TestInverseAndSolve(t *testing.T) {
	a := [][]float64{{4, 7}, {2, 6}}
	inv, ok := inverse(a)
	if !ok {
		t.Fatal("expected invertible matrix")
	}
	// A * A⁻¹ should be the identity.
	prod, _ := matmul(a, inv)
	eqMat(t, prod, identity(2), 1e-9, "A*inv")

	// Solve A x = b where the answer is known.
	x, ok := solve([][]float64{{2, 1}, {1, 3}}, []float64{5, 10})
	if !ok {
		t.Fatal("expected solvable system")
	}
	eqf(t, x[0], 1, 1e-9, "solve x0")
	eqf(t, x[1], 3, 1e-9, "solve x1")

	if _, ok := inverse([][]float64{{1, 2}, {2, 4}}); ok {
		t.Fatal("expected singular matrix to fail inverse")
	}
}
