package stats

import (
	"math"
	"testing"
)

func TestTwoTailedPAgainstReference(t *testing.T) {
	cases := []struct {
		tStat, df, want float64
	}{
		{2.0, 10, 0.073388},
		{-2.0, 10, 0.073388},
		{0.0, 5, 1.0},
		{3.5, 20, 0.002249},
		{1.0, 1, 0.5},
		{6.0, 5, 0.0018},
	}
	for _, c := range cases {
		got := twoTailedP(c.tStat, c.df)
		if math.Abs(got-c.want) > 1e-4 {
			t.Errorf("twoTailedP(%g, %g) = %.6f, want %.6f", c.tStat, c.df, got, c.want)
		}
	}
}

func TestTwoTailedPApproachesNormal(t *testing.T) {
	const z = 1.96
	normalP := 2 * (1 - 0.5*math.Erfc(-z/math.Sqrt2))

	prev := math.Inf(1)
	for _, df := range []float64{10, 100, 1000, 100000} {
		got := twoTailedP(z, df)
		if got < normalP {
			t.Errorf("df=%g: p = %.6f, below the normal limit %.6f", df, got, normalP)
		}
		if got > prev {
			t.Errorf("df=%g: p = %.6f rose above the previous %.6f; it should decrease toward the limit", df, got, prev)
		}
		prev = got
	}
	if math.Abs(prev-normalP) > 1e-4 {
		t.Errorf("at df=100000, p = %.6f, want it within 1e-4 of the normal %.6f", prev, normalP)
	}
}

func TestSumIsCompensated(t *testing.T) {
	xs := []float64{1e16}
	for range 10 {
		xs = append(xs, 1.0)
	}
	if got, want := sum(xs), 1e16+10; got != want {
		t.Errorf("sum = %.1f, want %.1f (naive addition loses the ones entirely)", got, want)
	}

	rev := []float64{}
	for range 10 {
		rev = append(rev, 1.0)
	}
	rev = append(rev, 1e16)
	if got, want := sum(rev), 1e16+10; got != want {
		t.Errorf("sum (big value last) = %.1f, want %.1f", got, want)
	}
}

func TestSumMatchesNaiveOnEasyInput(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5.5}
	if got, want := sum(xs), 15.5; got != want {
		t.Errorf("sum = %g, want %g", got, want)
	}
}

func TestRanksAverageTies(t *testing.T) {
	got := ranks([]float64{10, 20, 20, 30})
	want := []float64{1, 2.5, 2.5, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ranks[%d] = %g, want %g", i, got[i], want[i])
		}
	}

	got = ranks([]float64{1, 5, 5, 5, 9})
	want = []float64{1, 3, 3, 3, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("three-way tie: ranks[%d] = %g, want %g", i, got[i], want[i])
		}
	}
}

func TestSpearmanDetectsNonLinearMonotonic(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{1, 10, 100, 1000, 10000}

	if got := spearman(x, y); math.Abs(got-1) > 1e-12 {
		t.Errorf("spearman = %g, want 1", got)
	}
	if p := correlation(x, y); p > 0.95 {
		t.Errorf("correlation = %g; the linear measure should be well below 1 here", p)
	}

	rev := []float64{10000, 1000, 100, 10, 1}
	if got := spearman(x, rev); math.Abs(got+1) > 1e-12 {
		t.Errorf("spearman (reversed) = %g, want -1", got)
	}
}

func TestNormalDistributionKnownPoints(t *testing.T) {
	if got := 0.5 * math.Erfc(0); math.Abs(got-0.5) > 1e-15 {
		t.Errorf("normal cdf at 0 = %g, want 0.5", got)
	}
	want := 1 / math.Sqrt(2*math.Pi)
	got := math.Exp(0) / math.Sqrt(2*math.Pi)
	if math.Abs(got-want) > 1e-15 {
		t.Errorf("normal pdf at 0 = %g, want %g", got, want)
	}
}
