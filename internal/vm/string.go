package vm

// concatOne renders a single Lua value for string concatenation. Strings pass
// through; numbers use their canonical Lua textual form; everything else
// (after metamethod lookup has already failed) raises a runtime error.
func concatOne(v Value) string {
	switch v := v.(type) {
	case string:
		return v
	case int64, float64:
		return ToString(v)
	}
	panic(Errorf("attempt to concatenate a %s value", TypeName(v)))
}

// concatPair joins two values per Lua semantics (numbers and strings only).
// The variadic Concat opcode in the VM handles the n-ary case directly.
func concatPair(a, b Value) string {
	return concatOne(a) + concatOne(b)
}
