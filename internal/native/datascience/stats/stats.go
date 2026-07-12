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

func sum(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s
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
