package vm

import "strconv"

// formatInteger renders a Lua integer (int64) using base-10.
func formatInteger(x int64) string {
	return strconv.FormatInt(x, 10)
}

// Small integers are interned into a shared table of pre-boxed Values. On a
// 32-bit build (GOARCH=386) every int64 → Value(any) conversion otherwise
// heap-allocates, and loop counters / small-magnitude arithmetic dominate
// real workloads. Interned boxes are immutable and Lua compares numbers by
// value, so handing out a shared box is fully transparent.
const (
	internIntMin = -256
	internIntMax = 8191
)

var internedInts [internIntMax - internIntMin + 1]Value

func init() {
	for i := range internedInts {
		internedInts[i] = int64(internIntMin + i)
	}
}

// internInt boxes n into a Value, reusing a cached box for small magnitudes
// and falling back to a fresh boxing conversion outside the cached range.
func internInt(n int64) Value {
	if n >= internIntMin && n <= internIntMax {
		return internedInts[n-internIntMin]
	}
	return n
}

// intArith performs +, -, *, % on two integers, preserving the integer
// subtype. Mod follows Lua's "result has the sign of b" semantics. Results
// route through internInt so hot arithmetic avoids per-op boxing allocs.
func intArith(a, b int64, op string) Value {
	switch op {
	case "+":
		return internInt(a + b)
	case "-":
		return internInt(a - b)
	case "*":
		return internInt(a * b)
	case "%":
		if b == 0 {
			panic(LuaError("attempt to perform 'n%0'"))
		}
		r := a % b
		if r != 0 && (r^b) < 0 {
			r += b
		}
		return internInt(r)
	}
	panic("internal: intArith op " + op)
}
