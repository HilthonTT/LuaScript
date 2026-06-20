package math

import "math"

func deg(x float64) float64 {
	return x * (180 / math.Pi)
}

func rad(x float64) float64 {
	return x * (math.Pi / 180)
}

// softmax converts a slice of scores into a probability distribution. The
// max is subtracted before exponentiating for numerical stability (so large
// inputs don't overflow Exp). An empty input yields an empty output rather
// than dividing by a zero sum.
func softmax(xx []float64) []float64 {
	if len(xx) == 0 {
		return []float64{}
	}

	out := make([]float64, len(xx))
	var sum float64
	m := maxOf(xx)
	for i, x := range xx {
		out[i] = math.Exp(x - m)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// mean returns the arithmetic mean. Callers must guarantee a non-empty slice;
// mean(nil) would divide by zero.
func mean(xx []float64) float64 {
	var sum float64
	for _, x := range xx {
		sum += x
	}
	return sum / float64(len(xx))
}

// variance returns the sample variance (Bessel-corrected, dividing by n-1).
// Fewer than two samples have no spread, so we return 0 rather than dividing
// by zero or a negative count.
func variance(xx []float64) float64 {
	if len(xx) < 2 {
		return 0.0
	}

	m := mean(xx)

	var variance float64
	for _, x := range xx {
		variance += (x - m) * (x - m)
	}

	return variance / float64(len(xx)-1)
}

// standardDeviation is the square root of the sample variance.
func standardDeviation(xx []float64) float64 {
	return math.Sqrt(variance(xx))
}

// maxOf returns the largest element. Callers must guarantee a non-empty slice.
func maxOf(xx []float64) float64 {
	m := xx[0]
	for _, x := range xx[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

// minOf returns the smallest element. Callers must guarantee a non-empty slice.
func minOf(xx []float64) float64 {
	m := xx[0]
	for _, x := range xx[1:] {
		if x < m {
			m = x
		}
	}
	return m
}
