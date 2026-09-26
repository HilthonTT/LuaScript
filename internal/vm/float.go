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
	// strconv follows Go literal syntax, which Lua does not: "_" digit
	// separators and "inf"/"nan" spellings must not convert.
	if strings.ContainsAny(s, "nN_") {
		return 0, 0, false, false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "0x") || strings.HasPrefix(lower, "-0x") || strings.HasPrefix(lower, "+0x") {
		if i, ok := parseHexInt(lower); ok {
			return i, 0, true, true
		}
		// Go insists on a binary exponent in hex floats; Lua does not.
		if !strings.Contains(lower, "p") {
			s += "p0"
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

// parseHexInt parses a lower-cased, optionally signed "0x..." integer. Like
// Lua (and the lexer), it wraps around modulo 2^64 instead of overflowing,
// so "0xffffffffffffffff" is -1.
func parseHexInt(s string) (int64, bool) {
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	digits := s[2:]
	if digits == "" {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		switch {
		case c >= '0' && c <= '9':
			n = n<<4 | uint64(c-'0')
		case c >= 'a' && c <= 'f':
			n = n<<4 | uint64(c-'a'+10)
		default:
			return 0, false
		}
	}
	if neg {
		n = -n
	}
	return int64(n), true
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
