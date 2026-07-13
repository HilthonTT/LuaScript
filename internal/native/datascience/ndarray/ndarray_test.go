package ndarray

import (
	"math"
	"testing"
)

func nd(shape []int, data ...float64) *ndarray {
	a := newND(shape)
	copy(a.data, data)
	return a
}

func eqData(t *testing.T, got *ndarray, want ...float64) {
	t.Helper()
	if len(got.data) != len(want) {
		t.Fatalf("length: got %d, want %d (%v)", len(got.data), len(want), got.data)
	}
	for i := range want {
		if math.Abs(got.data[i]-want[i]) > 1e-9 {
			t.Fatalf("data[%d]: got %v, want %v (full %v)", i, got.data[i], want[i], got.data)
		}
	}
}

func TestBroadcastShapes(t *testing.T) {
	cases := []struct {
		a, b, want []int
		ok         bool
	}{
		{[]int{2, 3}, []int{3}, []int{2, 3}, true},    // row broadcast
		{[]int{2, 3}, []int{}, []int{2, 3}, true},     // scalar
		{[]int{2, 1}, []int{1, 3}, []int{2, 3}, true}, // outer
		{[]int{2, 3}, []int{4}, nil, false},           // mismatch
	}
	for _, c := range cases {
		out, ok := broadcast(c.a, c.b)
		if ok != c.ok {
			t.Fatalf("broadcast(%v,%v) ok=%v want %v", c.a, c.b, ok, c.ok)
		}
		if ok && !intsEqual(out, c.want) {
			t.Fatalf("broadcast(%v,%v)=%v want %v", c.a, c.b, out, c.want)
		}
	}
}

func TestEwiseScalarAndRowBroadcast(t *testing.T) {
	a := nd([]int{2, 3}, 1, 2, 3, 4, 5, 6)

	got := ewise("+", a, scalarND(10), func(x, y float64) float64 { return x + y })
	eqData(t, got, 11, 12, 13, 14, 15, 16)

	row := nd([]int{3}, 100, 200, 300)
	got = ewise("+", a, row, func(x, y float64) float64 { return x + y })
	eqData(t, got, 101, 202, 303, 104, 205, 306)
}

func TestReduceAllAndAxis(t *testing.T) {
	a := nd([]int{2, 3}, 1, 2, 3, 4, 5, 6)

	if s := a.reduceAll(sumf); s != 21 {
		t.Fatalf("sum all: got %v want 21", s)
	}
	// axis 0 -> collapse rows -> length-3 vector of column sums
	col := a.alongAxis("sum", 0, sumf)
	if !intsEqual(col.shape, []int{3}) {
		t.Fatalf("axis0 shape %v", col.shape)
	}
	eqData(t, col, 5, 7, 9)
	// axis 1 -> collapse cols -> length-2 vector of row means
	rm := a.alongAxis("mean", 1, meanf)
	eqData(t, rm, 2, 5)
}

func TestReshapeInferAndTranspose(t *testing.T) {
	a := nd([]int{2, 3}, 1, 2, 3, 4, 5, 6)

	r := a.reshape("reshape", []int{3, -1})
	if !intsEqual(r.shape, []int{3, 2}) {
		t.Fatalf("reshape infer shape %v", r.shape)
	}
	eqData(t, r, 1, 2, 3, 4, 5, 6)

	tp := a.transpose("transpose", nil)
	if !intsEqual(tp.shape, []int{3, 2}) {
		t.Fatalf("transpose shape %v", tp.shape)
	}
	eqData(t, tp, 1, 4, 2, 5, 3, 6)
}

func TestMatmul(t *testing.T) {
	a := nd([]int{2, 3}, 1, 2, 3, 4, 5, 6)
	b := nd([]int{3, 2}, 7, 8, 9, 10, 11, 12)
	got := matmul(a, b)
	if !intsEqual(got.shape, []int{2, 2}) {
		t.Fatalf("matmul shape %v", got.shape)
	}
	// [1 2 3;4 5 6] · [7 8;9 10;11 12] = [58 64; 139 154]
	eqData(t, got, 58, 64, 139, 154)

	// vector dot collapses to a 0-D scalar
	v1 := nd([]int{3}, 1, 2, 3)
	v2 := nd([]int{3}, 4, 5, 6)
	dot := matmul(v1, v2)
	if dot.ndim() != 0 || dot.data[0] != 32 {
		t.Fatalf("dot: ndim=%d val=%v", dot.ndim(), dot.data[0])
	}
}

func TestReshapeMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on bad reshape")
		}
	}()
	nd([]int{2, 3}, 1, 2, 3, 4, 5, 6).reshape("reshape", []int{4, 2})
}

func TestRender2D(t *testing.T) {
	a := nd([]int{2, 2}, 1, 2, 3, 4)
	got := a.render()
	want := "[[1 2],\n [3 4]]"
	if got != want {
		t.Fatalf("render:\n%q\nwant\n%q", got, want)
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
