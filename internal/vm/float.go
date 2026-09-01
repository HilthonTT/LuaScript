package vm

import (
	"math"
	"strconv"
	"strings"
)

func formatFloat(x float64) string {
	if math.IsNaN(x) {
		return "-nan"
	}
	if math.IsInf(x, 1) {
		return "inf"
	}
	if math.IsInf(x, -1) {
		return "-inf"
	}
	s := strconv.FormatFloat(x, 'g', 14, 64)
	if !strings.ContainsAny(s, ".eEnN") {
		s += ".0"
	}
	return s
}

func floatToInt(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if f != math.Trunc(f) {
		return 0, false
	}
	if f < -9.2233720368547758e+18 || f >= 9.2233720368547758e+18 {
		return 0, false
	}
	return int64(f), true
}

func parseNumber(s string) (int64, float64, bool, bool) {
	if s == "" {
		return 0, 0, false, false
	}
	if strings.ContainsAny(s, "nN") {
		return 0, 0, false, false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "0x") || strings.HasPrefix(lower, "-0x") || strings.HasPrefix(lower, "+0x") {
		if i, err := strconv.ParseInt(s, 0, 64); err == nil {
			return i, 0, true, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return 0, f, false, true
		}
		return 0, 0, false, false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, 0, true, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return 0, f, false, true
	}
	return 0, 0, false, false
}

func floatArith(a, b float64, op string) Value {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "%":
		m := math.Mod(a, b)
		if m != 0 && (m < 0) != (b < 0) {
			m += b
		}
		return m
	}
	panic("internal: floatArith op " + op)
}
