package vm

import (
	"math"
	"strconv"
	"strings"
)

// formatFloat renders a Lua float (float64). Floats that happen to be exactly
// integral receive a trailing ".0" so they remain visually distinguishable
// from integers (Lua's standard tostring does this too).
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

// floatToInt converts an exactly-integral float to int64 if it fits.
func floatToInt(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if f != math.Trunc(f) {
		return 0, false
	}
	// math.MaxInt64 doesn't roundtrip exactly through float64; use the
	// nearest representable bound.
	if f < -9.2233720368547758e+18 || f >= 9.2233720368547758e+18 {
		return 0, false
	}
	return int64(f), true
}

// parseNumber parses a Lua numeric literal: decimal int, decimal float, hex
// int (0x...), or hex float (0x...p±n). Empty input fails. Used by ToNumber
// when coercing a string to a number.
func parseNumber(s string) (int64, float64, bool, bool) {
	if s == "" {
		return 0, 0, false, false
	}
	// Lua rejects "inf"/"nan"/"infinity": no valid numeral contains 'n'/'N',
	// so refuse them before strconv.ParseFloat (which would accept them).
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

// floatArith performs +, -, *, % on two floats.
func floatArith(a, b float64, op string) Value {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "%":
		// Lua float modulo is fmod with a sign correction so the result takes
		// the sign of the divisor (floored modulo). Using math.Mod (fmod) rather
		// than the algebraic a-floor(a/b)*b is essential for infinite divisors:
		// math.Mod(x, ±Inf) == x, so `5 % math.huge` yields 5, whereas the old
		// formula computed 5 - 0*Inf = NaN.
		m := math.Mod(a, b)
		if m != 0 && (m < 0) != (b < 0) {
			m += b
		}
		return m
	}
	panic("internal: floatArith op " + op)
}
