package native

import "math"

func Sumf(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s
}

func Meanf(xs []float64) float64 {
	return Sumf(xs) / float64(len(xs))
}

func Variancef(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := Meanf(xs)
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return ss / float64(len(xs)-1)
}

func Stddevf(xs []float64) float64 {
	return math.Sqrt(Variancef(xs))
}

func Minf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func Maxf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

// ArgMax is the index of the largest element
func ArgMax(xs []float64) int {
	max, idx := xs[0], 0
	for i, x := range xs {
		if x > max {
			max, idx = xs[i], i
		}
	}
	return idx
}

// softmax converts a slice of scores into a probability distribution. The
// max is subtracted before exponentiating for numerical stability (so large
// inputs don't overflow Exp). An empty input yields an empty output rather
// than dividing by a zero sum.
func Softmax(xx []float64) []float64 {
	if len(xx) == 0 {
		return []float64{}
	}

	out := make([]float64, len(xx))
	var sum float64
	m := MaxOf(xx)
	for i, x := range xx {
		out[i] = math.Exp(x - m)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// maxOf returns the largest element. Callers must guarantee a non-empty slice.
func MaxOf(xx []float64) float64 {
	m := xx[0]
	for _, x := range xx[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

// minOf returns the smallest element. Callers must guarantee a non-empty slice.
func MinOf(xx []float64) float64 {
	m := xx[0]
	for _, x := range xx[1:] {
		if x < m {
			m = x
		}
	}
	return m
}
