package stats

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want, tol float64, name string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %g, want %g", name, got, want)
	}
}

func TestCentralTendency(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	approx(t, mean(xs), 5, 1e-9, "mean")
	approx(t, median(xs), 4.5, 1e-9, "median")
	approx(t, mode(xs), 4, 1e-9, "mode")
}

func TestSpread(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	// Population stddev of this classic example is 2.0; sample is larger.
	approx(t, pstddev(xs), 2.0, 1e-9, "pstddev")
	approx(t, pvariance(xs), 4.0, 1e-9, "pvariance")
	approx(t, variance(xs), 32.0/7.0, 1e-9, "variance")
	approx(t, rangeOf(xs), 7, 1e-9, "range")
}

func TestQuantiles(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	s := sortedCopy(xs)
	approx(t, quantileSorted(s, 0), 1, 1e-9, "q0")
	approx(t, quantileSorted(s, 1), 10, 1e-9, "q1.0")
	approx(t, quantileSorted(s, 0.5), 5.5, 1e-9, "median")
	// numpy linear interpolation: 25th percentile of 1..10 is 3.25.
	approx(t, quantileSorted(s, 0.25), 3.25, 1e-9, "q0.25")
	approx(t, iqr(xs), 4.5, 1e-9, "iqr")
}

func TestRelationships(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	b := []float64{2, 4, 6, 8, 10} // perfectly correlated, slope 2
	approx(t, correlation(a, b), 1, 1e-9, "correlation")
	c := []float64{10, 8, 6, 4, 2} // perfectly anti-correlated
	approx(t, correlation(a, c), -1, 1e-9, "anti-correlation")
	approx(t, covariance(a, b), 5, 1e-9, "covariance")
}

func TestTransforms(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5}
	z := zscore(xs)
	approx(t, mean(z), 0, 1e-9, "zscore mean")
	approx(t, stddev(z), 1, 1e-9, "zscore stddev")

	n := normalize(xs)
	approx(t, n[0], 0, 1e-9, "normalize min")
	approx(t, n[len(n)-1], 1, 1e-9, "normalize max")

	cs := cumsum(xs)
	approx(t, cs[len(cs)-1], 15, 1e-9, "cumsum total")
}

func TestMeansAndShape(t *testing.T) {
	xs := []float64{1, 2, 4, 8}
	approx(t, geomean(xs), math.Pow(64, 0.25), 1e-9, "geomean")
	// Harmonic mean of 1,2,4,8 = 4 / (1 + 0.5 + 0.25 + 0.125)
	approx(t, harmonicMean(xs), 4/1.875, 1e-9, "harmonic_mean")

	// Data symmetric about its mean has zero skew.
	sym := []float64{1, 2, 3, 4, 5, 6, 7}
	approx(t, skewness(sym), 0, 1e-9, "skewness symmetric")
}

func TestConstantArrayNoNaN(t *testing.T) {
	xs := []float64{3, 3, 3, 3}
	for _, v := range zscore(xs) {
		if math.IsNaN(v) {
			t.Fatal("zscore of constant array produced NaN")
		}
	}
	for _, v := range normalize(xs) {
		if math.IsNaN(v) {
			t.Fatal("normalize of constant array produced NaN")
		}
	}
	if math.IsNaN(correlation(xs, xs)) {
		t.Fatal("correlation of constant array produced NaN")
	}
}
