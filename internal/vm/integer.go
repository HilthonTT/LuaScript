package vm

import "strconv"

func formatInteger(x int64) string {
	return strconv.FormatInt(x, 10)
}

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

func internInt(n int64) Value {
	if n >= internIntMin && n <= internIntMax {
		return internedInts[n-internIntMin]
	}
	return n
}

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
