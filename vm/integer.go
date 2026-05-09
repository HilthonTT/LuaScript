package vm

import "strconv"

// formatInteger renders a Lua integer (int64) using base-10.
func formatInteger(x int64) string {
	return strconv.FormatInt(x, 10)
}

// intArith performs +, -, *, % on two integers, preserving the integer
// subtype. Mod follows Lua's "result has the sign of b" semantics.
func intArith(a, b int64, op string) Value {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "%":
		if b == 0 {
			panic(LuaError("attempt to perform 'n%0'"))
		}
		r := a % b
		if r != 0 && (r^b) < 0 {
			r += b
		}
		return r
	}
	panic("internal: intArith op " + op)
}
