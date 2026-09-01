package vm

func concatOne(v Value) string {
	switch v := v.(type) {
	case string:
		return v
	case int64, float64:
		return ToString(v)
	}
	panic(Errorf("attempt to concatenate a %s value", TypeName(v)))
}

func concatPair(a, b Value) string {
	return concatOne(a) + concatOne(b)
}
