// Package stats is a require()-able host module providing the descriptive
// and inferential statistics a data-science workflow reaches for first:
// central tendency (mean/median/mode), spread (variance/stddev/iqr/range),
// shape (skewness/kurtosis), relationships (covariance/correlation), and
// the per-element transforms (zscore/normalize/standardize) used to prepare
// features. Every function takes a Lua array of numbers; the relationship
// functions take two equal-length arrays.
//
// The numeric core lives in functions over []float64 (tested directly in
// stats_test.go); the GoFunc wrappers only marshal Lua tables in and out.
package stats

import (
	"math"
	"sort"

	"github.com/hilthontt/luascript/internal/native"
	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterStatsPreload installs the loader under package.preload so the
// module table is built lazily on the first require("stats").
func RegisterStatsPreload(v *vm.VM) {
	vm.RegisterPreload(v, "stats", statsLoader)
}

func statsLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 8)
	methods := vm.NewTable(0, 32)

	// reducer registers a func that takes one numeric array and returns a scalar.
	reducer := func(name string, fn func([]float64) float64) {
		methods.Set(name, &vm.GoFunc{Name: "stats:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			xs := floats("stats."+name, vm.TableArg("stats."+name, 1, args))
			requireNonEmpty("stats."+name, xs)
			return []vm.Value{fn(xs)}
		}})
	}

	// mapper registers a func that maps one numeric array to another array.
	mapper := func(name string, fn func([]float64) []float64) {
		methods.Set(name, &vm.GoFunc{Name: "stats:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			xs := floats("stats."+name, vm.TableArg("stats."+name, 1, args))
			requireNonEmpty("stats."+name, xs)
			return []vm.Value{floatSliceToTable(fn(xs))}
		}})
	}

	// pair registers a func over two equal-length numeric arrays.
	pair := func(name string, fn func(a, b []float64) float64) {
		methods.Set(name, &vm.GoFunc{Name: "stats:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			a := floats("stats."+name, vm.TableArg("stats."+name, 1, args))
			b := floats("stats."+name, vm.TableArg("stats."+name, 2, args))
			if len(a) != len(b) {
				panic(vm.Errorf("stats.%s: arrays must be the same length (%d vs %d)", name, len(a), len(b)))
			}
			requireNonEmpty("stats."+name, a)
			return []vm.Value{fn(a, b)}
		}})
	}

	reducer("sum", sum)
	reducer("product", product)
	reducer("mean", mean)
	reducer("median", median)
	reducer("mode", mode)
	reducer("min", minOf)
	reducer("max", maxOf)
	reducer("range", rangeOf)
	reducer("variance", variance)   // sample (n-1)
	reducer("pvariance", pvariance) // population (n)
	reducer("stddev", stddev)       // sample
	reducer("pstddev", pstddev)     // population
	reducer("sem", sem)             // standard error of the mean
	reducer("iqr", iqr)             // interquartile range
	reducer("skewness", skewness)   // sample skewness
	reducer("kurtosis", kurtosis)   // excess kurtosis
	reducer("geomean", geomean)     // geometric mean
	reducer("harmonic_mean", harmonicMean)

	mapper("zscore", zscore)
	mapper("normalize", normalize) // min-max to [0, 1]
	mapper("standardize", zscore)  // alias: zero mean, unit variance
	mapper("cumsum", cumsum)

	pair("covariance", covariance)
	pair("correlation", correlation)
	// Spearman is Pearson on the ranks, so it measures any monotonic
	// relationship rather than only a linear one, and is unmoved by outliers
	// that would drag `correlation` around.
	pair("spearman", spearman)

	// weighted_mean(values, weights) — for averaging measurements that do not
	// carry equal confidence, which a plain mean cannot express.
	methods.Set("weighted_mean", &vm.GoFunc{Name: "stats:weighted_mean", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		xs := floats("stats.weighted_mean", vm.TableArg("stats.weighted_mean", 1, args))
		ws := floats("stats.weighted_mean", vm.TableArg("stats.weighted_mean", 2, args))
		if len(xs) != len(ws) {
			panic(vm.Errorf("stats.weighted_mean: arrays must be the same length (%d vs %d)", len(xs), len(ws)))
		}
		requireNonEmpty("stats.weighted_mean", xs)
		products := make([]float64, len(xs))
		for i := range xs {
			products[i] = xs[i] * ws[i]
		}
		total := sum(ws)
		if total == 0 {
			panic(vm.Errorf("stats.weighted_mean: weights sum to zero"))
		}
		return []vm.Value{sum(products) / total}
	}})

	// histogram(values, bins) -> { counts, edges }. Binning by hand in script
	// code is easy to get subtly wrong at the top edge, which belongs in the
	// last bin rather than in a bin of its own.
	methods.Set("histogram", &vm.GoFunc{Name: "stats:histogram", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		xs := floats("stats.histogram", vm.TableArg("stats.histogram", 1, args))
		requireNonEmpty("stats.histogram", xs)
		bins := int(vm.OptInt("stats.histogram", 2, args, 10))
		if bins < 1 {
			panic(vm.Errorf("stats.histogram: bins must be >= 1, got %d", bins))
		}
		lo, hi := minOf(xs), maxOf(xs)
		if lo == hi {
			// A degenerate range would give a zero-width bin and a division by
			// zero; widen it so every value lands in the middle bin.
			lo, hi = lo-0.5, hi+0.5
		}
		width := (hi - lo) / float64(bins)
		counts := make([]float64, bins)
		for _, x := range xs {
			idx := int((x - lo) / width)
			// The maximum lands exactly on the top edge; it belongs to the
			// last bin, not to a bin one past the end.
			if idx >= bins {
				idx = bins - 1
			}
			if idx < 0 {
				idx = 0
			}
			counts[idx]++
		}
		edges := make([]float64, bins+1)
		for i := range edges {
			edges[i] = lo + float64(i)*width
		}
		out := vm.NewTable(0, 2)
		out.Set("counts", floatSliceToTable(counts))
		out.Set("edges", floatSliceToTable(edges))
		return []vm.Value{out}
	}})

	// Normal distribution. Turning a z-score into a probability (and back)
	// is what every significance judgement needs, and neither is expressible
	// in script code without erf.
	methods.Set("normal_pdf", &vm.GoFunc{Name: "stats:normal_pdf", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("stats.normal_pdf", 1, args)
		mu := optFloat(args, 2, 0)
		sigma := optFloat(args, 3, 1)
		requirePositiveSigma("stats.normal_pdf", sigma)
		z := (x - mu) / sigma
		return []vm.Value{math.Exp(-0.5*z*z) / (sigma * math.Sqrt(2*math.Pi))}
	}})
	methods.Set("normal_cdf", &vm.GoFunc{Name: "stats:normal_cdf", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := vm.FloatArg("stats.normal_cdf", 1, args)
		mu := optFloat(args, 2, 0)
		sigma := optFloat(args, 3, 1)
		requirePositiveSigma("stats.normal_cdf", sigma)
		return []vm.Value{0.5 * math.Erfc(-(x-mu)/(sigma*math.Sqrt2))}
	}})

	// t_test_1sample(values, mu) -> { t, df, p }. Two-tailed.
	methods.Set("t_test_1sample", &vm.GoFunc{Name: "stats:t_test_1sample", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		xs := floats("stats.t_test_1sample", vm.TableArg("stats.t_test_1sample", 1, args))
		if len(xs) < 2 {
			panic(vm.Errorf("stats.t_test_1sample: need at least 2 values, got %d", len(xs)))
		}
		mu := optFloat(args, 2, 0)
		se := sem(xs)
		if se == 0 {
			panic(vm.Errorf("stats.t_test_1sample: sample has zero variance"))
		}
		tStat := (mean(xs) - mu) / se
		df := float64(len(xs) - 1)
		return []vm.Value{tTestResult(tStat, df)}
	}})

	// t_test_2sample(a, b) -> { t, df, p }. Welch's version: it does not
	// assume the two groups share a variance, which is the assumption most
	// often violated in practice.
	methods.Set("t_test_2sample", &vm.GoFunc{Name: "stats:t_test_2sample", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		a := floats("stats.t_test_2sample", vm.TableArg("stats.t_test_2sample", 1, args))
		b := floats("stats.t_test_2sample", vm.TableArg("stats.t_test_2sample", 2, args))
		if len(a) < 2 || len(b) < 2 {
			panic(vm.Errorf("stats.t_test_2sample: each sample needs at least 2 values (got %d and %d)", len(a), len(b)))
		}
		va, vb := variance(a)/float64(len(a)), variance(b)/float64(len(b))
		if va+vb == 0 {
			panic(vm.Errorf("stats.t_test_2sample: both samples have zero variance"))
		}
		tStat := (mean(a) - mean(b)) / math.Sqrt(va+vb)
		// Welch–Satterthwaite degrees of freedom.
		df := (va + vb) * (va + vb) /
			(va*va/float64(len(a)-1) + vb*vb/float64(len(b)-1))
		return []vm.Value{tTestResult(tStat, df)}
	}})

	// quantile(t, q) with q in [0, 1] — linear interpolation (numpy/R type 7).
	methods.Set("quantile", &vm.GoFunc{Name: "stats:quantile", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		xs := floats("stats.quantile", vm.TableArg("stats.quantile", 1, args))
		requireNonEmpty("stats.quantile", xs)
		q := vm.FloatArg("stats.quantile", 2, args)
		if q < 0 || q > 1 {
			panic(vm.Errorf("stats.quantile: q must be in [0, 1], got %g", q))
		}
		return []vm.Value{quantileSorted(sortedCopy(xs), q)}
	}})

	// percentile(t, p) with p in [0, 100] — a friendlier face on quantile.
	methods.Set("percentile", &vm.GoFunc{Name: "stats:percentile", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		xs := floats("stats.percentile", vm.TableArg("stats.percentile", 1, args))
		requireNonEmpty("stats.percentile", xs)
		p := vm.FloatArg("stats.percentile", 2, args)
		if p < 0 || p > 100 {
			panic(vm.Errorf("stats.percentile: p must be in [0, 100], got %g", p))
		}
		return []vm.Value{quantileSorted(sortedCopy(xs), p/100)}
	}})

	// describe(t) -> { count, mean, std, min, q1, median, q3, max } — the
	// one-call summary you reach for before plotting anything.
	methods.Set("describe", &vm.GoFunc{Name: "stats:describe", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		xs := floats("stats.describe", vm.TableArg("stats.describe", 1, args))
		requireNonEmpty("stats.describe", xs)
		s := sortedCopy(xs)
		out := vm.NewTable(0, 8)
		out.Set("count", int64(len(xs)))
		out.Set("mean", mean(xs))
		out.Set("std", stddev(xs))
		out.Set("min", s[0])
		out.Set("q1", quantileSorted(s, 0.25))
		out.Set("median", quantileSorted(s, 0.5))
		out.Set("q3", quantileSorted(s, 0.75))
		out.Set("max", s[len(s)-1])
		return []vm.Value{out}
	}})

	m.Set("VERSION", "0.1.0")
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return []vm.Value{m}
}

// Numeric core — pure functions over []float64. Callers guarantee non-empty
// input (the wrappers enforce it) unless a function documents otherwise.

// sum adds with Neumaier compensation rather than a naive running total.
//
// Plain accumulation loses the low bits of every addend that is small relative
// to the total so far, and the error grows with the array. The classic
// demonstration is summing 1e16 with ten 1.0s: naive addition returns 1e16,
// having dropped every one of them. Since mean, variance and everything built
// on them route through here, the error would propagate into the whole module.
//
// Neumaier's variant is used instead of plain Kahan because it also stays
// exact when an addend is larger in magnitude than the running total.
func sum(xs []float64) float64 {
	var s, c float64
	for _, x := range xs {
		t := s + x
		if math.Abs(s) >= math.Abs(x) {
			// s is larger: the low bits of x are what get lost.
			c += (s - t) + x
		} else {
			c += (x - t) + s
		}
		s = t
	}
	return s + c
}

func product(xs []float64) float64 {
	p := 1.0
	for _, x := range xs {
		p *= x
	}
	return p
}

func mean(xs []float64) float64 {
	return sum(xs) / float64(len(xs))
}

// median returns the middle value (mean of the two middle values for an even
// count), computed on a sorted copy so the caller's array is left untouched.
func median(xs []float64) float64 {
	return quantileSorted(sortedCopy(xs), 0.5)
}

// mode returns the most frequently occurring value. Ties are broken by the
// smallest value, so the result is deterministic.
func mode(xs []float64) float64 {
	counts := make(map[float64]int, len(xs))
	for _, x := range xs {
		counts[x]++
	}
	best, bestCount := xs[0], 0
	for v, c := range counts {
		if c > bestCount || (c == bestCount && v < best) {
			best, bestCount = v, c
		}
	}
	return best
}

func minOf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxOf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func rangeOf(xs []float64) float64 {
	return maxOf(xs) - minOf(xs)
}

// sumSquaredDev returns Σ(x-mean)², the shared core of variance and stddev.
func sumSquaredDev(xs []float64) float64 {
	m := mean(xs)
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return ss
}

// variance is the sample variance (Bessel-corrected, divides by n-1). A single
// sample has no spread, so it returns 0 rather than dividing by zero.
func variance(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	return sumSquaredDev(xs) / float64(len(xs)-1)
}

// pvariance is the population variance (divides by n).
func pvariance(xs []float64) float64 {
	return sumSquaredDev(xs) / float64(len(xs))
}

func stddev(xs []float64) float64  { return math.Sqrt(variance(xs)) }
func pstddev(xs []float64) float64 { return math.Sqrt(pvariance(xs)) }

// sem is the standard error of the mean: stddev / sqrt(n).
func sem(xs []float64) float64 {
	return stddev(xs) / math.Sqrt(float64(len(xs)))
}

// iqr is the interquartile range, Q3 - Q1.
func iqr(xs []float64) float64 {
	s := sortedCopy(xs)
	return quantileSorted(s, 0.75) - quantileSorted(s, 0.25)
}

// skewness is the sample (adjusted Fisher-Pearson) skewness. It needs at least
// three points and non-zero spread; degenerate input yields 0.
func skewness(xs []float64) float64 {
	n := float64(len(xs))
	if n < 3 {
		return 0
	}
	m, sd := mean(xs), pstddev(xs)
	if sd == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		d := (x - m) / sd
		s += d * d * d
	}
	return (n / ((n - 1) * (n - 2))) * s
}

// kurtosis is the sample excess kurtosis (0 for a normal distribution). It
// needs at least four points and non-zero spread.
func kurtosis(xs []float64) float64 {
	n := float64(len(xs))
	if n < 4 {
		return 0
	}
	m, sd := mean(xs), pstddev(xs)
	if sd == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		d := (x - m) / sd
		s += d * d * d * d
	}
	a := (n * (n + 1)) / ((n - 1) * (n - 2) * (n - 3))
	b := (3 * (n - 1) * (n - 1)) / ((n - 2) * (n - 3))
	return a*s - b
}

// geomean is the geometric mean; all values must be positive.
func geomean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		if x <= 0 {
			panic(vm.Errorf("stats.geomean: all values must be positive (found %g)", x))
		}
		s += math.Log(x)
	}
	return math.Exp(s / float64(len(xs)))
}

// harmonicMean is the harmonic mean; all values must be positive.
func harmonicMean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		if x <= 0 {
			panic(vm.Errorf("stats.harmonic_mean: all values must be positive (found %g)", x))
		}
		s += 1 / x
	}
	return float64(len(xs)) / s
}

// zscore maps each element to (x-mean)/stddev. With zero spread every element
// maps to 0 rather than NaN.
func zscore(xs []float64) []float64 {
	m, sd := mean(xs), stddev(xs)
	out := make([]float64, len(xs))
	if sd == 0 {
		return out
	}
	for i, x := range xs {
		out[i] = (x - m) / sd
	}
	return out
}

// normalize scales each element into [0, 1] by min-max. A constant array (zero
// range) maps every element to 0.
func normalize(xs []float64) []float64 {
	lo, hi := minOf(xs), maxOf(xs)
	out := make([]float64, len(xs))
	if hi == lo {
		return out
	}
	for i, x := range xs {
		out[i] = (x - lo) / (hi - lo)
	}
	return out
}

func cumsum(xs []float64) []float64 {
	out := make([]float64, len(xs))
	var run float64
	for i, x := range xs {
		run += x
		out[i] = run
	}
	return out
}

// covariance is the sample covariance of two equal-length series.
func covariance(a, b []float64) float64 {
	if len(a) < 2 {
		return 0
	}
	ma, mb := mean(a), mean(b)
	var s float64
	for i := range a {
		s += (a[i] - ma) * (b[i] - mb)
	}
	return s / float64(len(a)-1)
}

// correlation is the Pearson correlation coefficient in [-1, 1]. If either
// series has zero variance the correlation is undefined and reported as 0.
func correlation(a, b []float64) float64 {
	sda, sdb := stddev(a), stddev(b)
	if sda == 0 || sdb == 0 {
		return 0
	}
	return covariance(a, b) / (sda * sdb)
}

// quantileSorted computes the q-quantile of an already-sorted slice using
// linear interpolation between order statistics (numpy's default, type 7).
func quantileSorted(s []float64, q float64) float64 {
	n := len(s)
	if n == 1 {
		return s[0]
	}
	pos := q * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return s[lo]
	}
	frac := pos - float64(lo)
	return s[lo]*(1-frac) + s[hi]*frac
}

// Marshalling helpers

func sortedCopy(xs []float64) []float64 {
	s := make([]float64, len(xs))
	copy(s, xs)
	sort.Float64s(s)
	return s
}

func requireNonEmpty(site string, xs []float64) {
	if len(xs) == 0 {
		panic(vm.Errorf("%s: expected a non-empty array of numbers", site))
	}
}

// floats reads a Lua array into a []float64, promoting integers to floats.
// A non-numeric array is rejected rather than silently zero-filled.
func floats(site string, t *vm.Table) []float64 {
	arr, err := native.ExtractArray(t)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}
	switch a := arr.(type) {
	case []int64:
		out := make([]float64, len(a))
		for i, v := range a {
			out[i] = float64(v)
		}
		return out
	case []float64:
		return a
	default:
		panic(vm.Errorf("%s: expected an array of numbers", site))
	}
}

func floatSliceToTable(xs []float64) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	native.WriteBack(t, xs)
	return t
}

// optFloat reads an optional numeric argument at position n (1-based).
func optFloat(args []vm.Value, n int, dflt float64) float64 {
	if n > len(args) || args[n-1] == nil {
		return dflt
	}
	f, ok := vm.ToFloat(args[n-1])
	if !ok {
		return dflt
	}
	return f
}

func requirePositiveSigma(site string, sigma float64) {
	if sigma <= 0 {
		panic(vm.Errorf("%s: sigma must be > 0, got %g", site, sigma))
	}
}

// spearman is Pearson's correlation computed on the ranks of each input, so it
// detects any monotonic relationship rather than only a linear one.
func spearman(a, b []float64) float64 {
	return correlation(ranks(a), ranks(b))
}

// ranks returns the 1-based rank of each element, averaging the ranks of tied
// values — the standard correction, without which ties would bias the result
// by the arbitrary order they happened to be stored in.
func ranks(xs []float64) []float64 {
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return xs[idx[i]] < xs[idx[j]] })

	out := make([]float64, len(xs))
	for i := 0; i < len(idx); {
		// Find the extent of this run of equal values.
		j := i
		for j+1 < len(idx) && xs[idx[j+1]] == xs[idx[i]] {
			j++
		}
		// Ranks are 1-based, so the run spans i+1..j+1; they all take its mean.
		avg := float64(i+1+j+1) / 2
		for k := i; k <= j; k++ {
			out[idx[k]] = avg
		}
		i = j + 1
	}
	return out
}

// tTestResult packages a t statistic with its degrees of freedom and the
// two-tailed p-value.
func tTestResult(tStat, df float64) *vm.Table {
	out := vm.NewTable(0, 3)
	out.Set("t", tStat)
	out.Set("df", df)
	out.Set("p", twoTailedP(tStat, df))
	return out
}

// twoTailedP is the two-tailed p-value of a t statistic, via the regularized
// incomplete beta function: p = I_{df/(df+t²)}(df/2, 1/2).
func twoTailedP(tStat, df float64) float64 {
	if df <= 0 || math.IsNaN(tStat) {
		return math.NaN()
	}
	return incompleteBeta(df/(df+tStat*tStat), df/2, 0.5)
}

// incompleteBeta is the regularized incomplete beta function I_x(a, b),
// evaluated with the continued fraction from Numerical Recipes. The symmetry
// I_x(a,b) = 1 - I_{1-x}(b,a) is used to keep x in the range where the
// fraction converges quickly.
func incompleteBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	front := math.Exp(lnBeta(a, b, x))
	if x < (a+1)/(a+b+2) {
		return front * betaCF(x, a, b) / a
	}
	return 1 - math.Exp(lnBeta(b, a, 1-x))*betaCF(1-x, b, a)/b
}

// lnBeta is the log of the leading factor x^a (1-x)^b / B(a, b), computed in
// log space so intermediate terms cannot overflow for large df.
func lnBeta(a, b, x float64) float64 {
	lgA, _ := math.Lgamma(a)
	lgB, _ := math.Lgamma(b)
	lgAB, _ := math.Lgamma(a + b)
	return lgAB - lgA - lgB + a*math.Log(x) + b*math.Log(1-x)
}

// betaCF evaluates the continued fraction for the incomplete beta function
// using the modified Lentz algorithm.
func betaCF(x, a, b float64) float64 {
	const (
		maxIter = 200
		epsilon = 3e-14
		tiny    = 1e-300
	)
	qab, qap, qam := a+b, a+1, a-1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		fm := float64(m)
		m2 := 2 * fm
		// Even step.
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		// Odd step.
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < epsilon {
			break
		}
	}
	return h
}
