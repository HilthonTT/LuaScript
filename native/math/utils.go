package math

import "math"

func deg(x float64) float64 {
	return x * (180 / math.Pi)
}

func rad(x float64) float64 {
	return x * (math.Pi / 180)
}
