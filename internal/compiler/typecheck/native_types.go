package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) requireModuleType(call *ast.CallExpression) *Type {
	ident, ok := call.Func.(*ast.Identifier)
	if !ok || ident.Name != "require" || len(call.Args) != 1 {
		return nil
	}
	lit, ok := call.Args[0].(*ast.StringLiteral)
	if !ok {
		return nil
	}
	if c.env.shadowsGlobal("require") {
		return nil
	}
	return nativeModules()[lit.Value]
}

func NativeModuleFields(name string) (map[string]bool, bool) {
	t, ok := nativeModules()[name]
	if !ok || t.Table == nil {
		return nil, false
	}
	out := make(map[string]bool, len(t.Table.Fields))
	for _, f := range t.Table.Fields {
		out[f.Key] = true
	}
	return out, true
}

func NativeFieldIsFunction(name, field string) (bool, bool) {
	t, ok := nativeModules()[name]
	if !ok || t.Table == nil {
		return false, false
	}
	for _, f := range t.Table.Fields {
		if f.Key == field {
			return f.Type != nil && f.Type.Kind == KindFunction, true
		}
	}
	return false, false
}

func NativeModuleNames() []string {
	mods := nativeModules()
	names := make([]string, 0, len(mods))
	for name := range mods {
		names = append(names, name)
	}
	return names
}

func nativeModules() map[string]*Type {
	m := map[string]*Type{}

	withVersion := func(fields []TableField) *Type {
		return NewTable(append(fields,
			TableField{Key: "VERSION", Type: stringT}), nil)
	}

	m["json"] = withVersion([]TableField{
		{Key: "encode", Type: NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{stringT}, false, nil)},
		{Key: "decode", Type: NewFunction([]*Type{stringT, Optional(anyT)}, []*Type{anyT}, false, nil)},
		{Key: "null", Type: anyT},
		{Key: "empty_array", Type: anyT},
	})

	m["crypto"] = withVersion([]TableField{
		{Key: "md5", Type: strToStr()},
		{Key: "sha1", Type: strToStr()},
		{Key: "sha3", Type: strToStr()},
		{Key: "sha256", Type: strToStr()},
		{Key: "sha512", Type: strToStr()},
		{Key: "hmac_sha256", Type: NewFunction([]*Type{stringT, stringT}, []*Type{stringT}, false, nil)},
		{Key: "hmac", Type: NewFunction([]*Type{stringT, stringT, stringT}, []*Type{stringT}, false, nil)},
		{Key: "hmac_verify", Type: NewFunction([]*Type{stringT, stringT, stringT}, []*Type{booleanT}, false, nil)},
		{Key: "constant_time_equal", Type: NewFunction([]*Type{stringT, stringT}, []*Type{booleanT}, false, nil)},
		{Key: "base64_encode", Type: strToStr()},
		{Key: "base64_decode", Type: strToStr()},
		{Key: "base64url_encode", Type: strToStr()},
		{Key: "base64url_decode", Type: strToStr()},
		{Key: "hex_encode", Type: strToStr()},
		{Key: "hex_decode", Type: strToStr()},
		{Key: "random_bytes", Type: NewFunction([]*Type{numberT}, []*Type{stringT}, false, nil)},
		{Key: "random_int", Type: NewFunction([]*Type{numberT}, []*Type{numberT}, false, nil)},
		{Key: "password_hash", Type: NewFunction([]*Type{stringT, Optional(anyT)},
			[]*Type{stringT}, false, nil)},
		{Key: "password_verify", Type: NewFunction([]*Type{stringT, stringT},
			[]*Type{booleanT}, false, nil)},
		{Key: "pbkdf2", Type: NewFunction(
			[]*Type{stringT, stringT, numberT, numberT, Optional(stringT)},
			[]*Type{stringT}, false, nil)},
	})

	m["regexp"] = withVersion([]TableField{
		{Key: "compile", Type: NewFunction([]*Type{stringT}, []*Type{regexObjectType()}, false, nil)},
		{Key: "quote", Type: strToStr()},
		{Key: "is_valid", Type: NewFunction([]*Type{stringT}, []*Type{booleanT}, false, nil)},
	})

	m["bit32"] = NewTable([]TableField{
		{Key: "band", Type: NewFunction(nil, []*Type{numberT}, true, numberT)},
		{Key: "bor", Type: NewFunction(nil, []*Type{numberT}, true, numberT)},
		{Key: "bxor", Type: NewFunction(nil, []*Type{numberT}, true, numberT)},
		{Key: "btest", Type: NewFunction(nil, []*Type{booleanT}, true, numberT)},
		{Key: "bnot", Type: numToNum()},
		{Key: "lshift", Type: num2ToNum()},
		{Key: "rshift", Type: num2ToNum()},
		{Key: "arshift", Type: num2ToNum()},
		{Key: "lrotate", Type: num2ToNum()},
		{Key: "rrotate", Type: num2ToNum()},
		{Key: "bswap", Type: numToNum()},
		{Key: "extract", Type: NewFunction([]*Type{numberT, numberT, Optional(numberT)},
			[]*Type{numberT}, false, nil)},
		{Key: "replace", Type: NewFunction([]*Type{numberT, numberT, numberT, Optional(numberT)},
			[]*Type{numberT}, false, nil)},
	}, nil)

	m["utf8"] = NewTable([]TableField{
		{Key: "charpattern", Type: stringT},
		{Key: "char", Type: NewFunction(nil, []*Type{stringT}, true, numberT)},
		{Key: "codepoint", Type: NewFunction([]*Type{stringT, Optional(numberT), Optional(numberT)},
			[]*Type{Optional(numberT)}, true, numberT)},
		{Key: "len", Type: NewFunction([]*Type{stringT, Optional(numberT), Optional(numberT)},
			[]*Type{Optional(numberT), Optional(numberT)}, false, nil)},
		{Key: "offset", Type: NewFunction([]*Type{stringT, numberT, Optional(numberT)},
			[]*Type{Optional(numberT)}, false, nil)},
		{Key: "codes", Type: NewFunction([]*Type{stringT}, []*Type{anyT, stringT, numberT}, false, nil)},
	}, nil)

	m["time"] = withVersion([]TableField{
		{Key: "RFC3339", Type: stringT},
		{Key: "DATE", Type: stringT},
		{Key: "DATETIME", Type: stringT},
		{Key: "KITCHEN", Type: stringT},
		{Key: "now", Type: NewFunction(nil, []*Type{numberT}, false, nil)},
		{Key: "now_ms", Type: NewFunction(nil, []*Type{numberT}, false, nil)},
		{Key: "clock", Type: NewFunction(nil, []*Type{numberT}, false, nil)},
		{Key: "sleep", Type: NewFunction([]*Type{numberT}, nil, false, nil)},
		{Key: "date", Type: NewFunction([]*Type{Optional(numberT), Optional(booleanT)},
			[]*Type{dateTableType()}, false, nil)},
		{Key: "format", Type: NewFunction([]*Type{numberT, Optional(stringT), Optional(booleanT)},
			[]*Type{stringT}, false, nil)},
		{Key: "parse", Type: NewFunction([]*Type{stringT, stringT}, []*Type{numberT}, false, nil)},
		{Key: "parse_utc", Type: NewFunction([]*Type{stringT, stringT}, []*Type{numberT}, false, nil)},
	})

	m["uuid"] = withVersion([]TableField{
		{Key: "v4", Type: NewFunction(nil, []*Type{stringT}, false, nil)},
		{Key: "is_valid", Type: NewFunction([]*Type{stringT}, []*Type{booleanT}, false, nil)},
	})

	m["sort"] = withVersion([]TableField{
		{Key: "sort", Type: sortFn()},
		{Key: "stable", Type: sortFn()},
		{Key: "quicksort", Type: sortFn()},
		{Key: "bubble", Type: sortFn()},
		{Key: "circle", Type: sortFn()},
		{Key: "simple", Type: sortFn()},
		{Key: "reverse", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
		{Key: "is_sorted", Type: NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{booleanT}, false, nil)},
	})

	m["compression"] = withVersion([]TableField{
		{Key: "encode", Type: strToStr()},
		{Key: "decode", Type: strToStr()},
		{Key: "symbol_count", Type: NewFunction([]*Type{stringT}, []*Type{anyT}, false, nil)},
		{Key: "codes", Type: NewFunction([]*Type{stringT}, []*Type{anyT}, false, nil)},
		{Key: "rle_encode", Type: strToStr()},
		{Key: "rle_decode", Type: strToStr()},
		{Key: "gzip_encode", Type: strToStr()},
		{Key: "gzip_decode", Type: strToStr()},
		{Key: "zlib_encode", Type: strToStr()},
		{Key: "zlib_decode", Type: strToStr()},
		{Key: "deflate_encode", Type: strToStr()},
		{Key: "deflate_decode", Type: strToStr()},
	})

	m["log"] = withVersion([]TableField{
		{Key: "LEVELS", Type: anyT},
		{Key: "trace", Type: logFn()},
		{Key: "debug", Type: logFn()},
		{Key: "info", Type: logFn()},
		{Key: "warn", Type: logFn()},
		{Key: "error", Type: logFn()},
		{Key: "fatal", Type: logFn()},
		{Key: "log", Type: NewFunction([]*Type{anyT}, nil, true, anyT)},
		{Key: "set_level", Type: NewFunction([]*Type{anyT}, nil, false, nil)},
		{Key: "get_level", Type: NewFunction(nil, []*Type{anyT}, false, nil)},
		{Key: "set_output", Type: NewFunction([]*Type{anyT}, nil, false, nil)},
		{Key: "get_output", Type: NewFunction(nil, []*Type{anyT}, false, nil)},
		{Key: "set_prefix", Type: NewFunction([]*Type{stringT}, nil, false, nil)},
		{Key: "close", Type: NewFunction(nil, nil, false, nil)},
	})

	m["debug"] = withVersion([]TableField{
		{Key: "traceback", Type: NewFunction([]*Type{Optional(stringT), Optional(numberT)},
			[]*Type{stringT}, false, nil)},
		{Key: "getinfo", Type: NewFunction([]*Type{anyT, Optional(stringT)}, []*Type{anyT}, false, nil)},
		{Key: "sethook", Type: NewFunction([]*Type{Optional(anyT), Optional(stringT), Optional(numberT)},
			nil, false, nil)},
		{Key: "gethook", Type: NewFunction(nil, []*Type{anyT}, false, nil)},
	})

	m["http"] = httpModuleType()
	m["os"] = osModuleType()

	ctor := func(names ...string) *Type {
		fields := make([]TableField, 0, len(names)+1)
		fields = append(fields, TableField{Key: "VERSION", Type: stringT})
		for _, n := range names {
			fields = append(fields, TableField{
				Key:  n,
				Type: NewFunction(nil, []*Type{anyT}, true, anyT),
			})
		}
		return NewTable(fields, nil)
	}

	m["std"] = ctor("new_btree", "new_deque", "new_hashmap", "new_heap", "new_list",
		"new_queue", "new_set", "new_stack", "new_trie")
	m["queue"] = ctor("new", "channel", "after", "tick")
	m["db"] = ctor("open", "drivers", "placeholder")
	m["httpserver"] = ctor("new")
	m["dataframe"] = ctor("new", "from_csv", "from_rows")
	m["ml"] = ctor("new", "load", "load_file")

	m["csv"] = withVersion([]TableField{
		{Key: "parse", Type: NewFunction([]*Type{stringT, Optional(anyT)}, []*Type{anyT}, false, nil)},
		{Key: "stringify", Type: NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{stringT}, false, nil)},
		{Key: "read", Type: NewFunction([]*Type{stringT, Optional(anyT)}, []*Type{anyT}, false, nil)},
		{Key: "write", Type: NewFunction([]*Type{stringT, anyT, Optional(anyT)}, []*Type{anyT}, false, nil)},
	})

	m["stats"] = statsModuleType()
	m["linalg"] = linalgModuleType()

	m["ndarray"] = withVersion([]TableField{
		{Key: "array", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
		{Key: "from_table", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
		{Key: "zeros", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "ones", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "full", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "eye", Type: NewFunction([]*Type{numberT}, []*Type{anyT}, false, nil)},
		{Key: "arange", Type: NewFunction([]*Type{numberT, Optional(numberT), Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		{Key: "linspace", Type: NewFunction([]*Type{numberT, numberT, Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		{Key: "concat", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "matmul", Type: NewFunction([]*Type{anyT, anyT}, []*Type{anyT}, false, nil)},
		{Key: "is_ndarray", Type: NewFunction([]*Type{anyT}, []*Type{booleanT}, false, nil)},
	})

	m["clustering"] = withVersion([]TableField{
		{Key: "kmeans", Type: fitFn()},
		{Key: "dbscan", Type: fitFn()},
		{Key: "hierarchical", Type: fitFn()},
		{Key: "meanshift", Type: fitFn()},
	})

	m["classification"] = withVersion([]TableField{
		{Key: "knn", Type: fitFn()},
		{Key: "logistic", Type: fitFn()},
		{Key: "naivebayes", Type: fitFn()},
		{Key: "perceptron", Type: fitFn()},
		{Key: "svm", Type: fitFn()},
	})

	m["plot"] = withVersion([]TableField{
		{Key: "figure", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "line", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "bar", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "scatter", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "histogram", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
	})

	m["plugin"] = NewTable([]TableField{
		{Key: "supported", Type: booleanT},
		{Key: "unsupported_reason", Type: NewFunction(nil, []*Type{stringT}, false, nil)},
		{Key: "dir", Type: NewFunction(nil, []*Type{stringT}, false, nil)},
		{Key: "open", Type: NewFunction([]*Type{stringT}, []*Type{anyT, Optional(stringT)}, false, nil)},
		{Key: "generate", Type: NewFunction([]*Type{stringT, Optional(anyT)},
			[]*Type{anyT, Optional(stringT)}, false, nil)},
	}, nil)

	m["test"] = testModuleType()
	m["math"] = nativeMathModuleType()

	return m
}

func fitFn() *Type {
	return NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{anyT}, false, nil)
}

func statsModuleType() *Type {
	reduce := func() *Type { return NewFunction([]*Type{anyT}, []*Type{numberT}, false, nil) }
	mapper := func() *Type { return NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil) }
	pair := func() *Type { return NewFunction([]*Type{anyT, anyT}, []*Type{numberT}, false, nil) }

	fields := []TableField{{Key: "VERSION", Type: stringT}}
	for _, n := range []string{
		"sum", "product", "mean", "median", "mode", "min", "max", "range",
		"variance", "pvariance", "stddev", "pstddev", "sem", "iqr",
		"skewness", "kurtosis", "geomean", "harmonic_mean",
	} {
		fields = append(fields, TableField{Key: n, Type: reduce()})
	}
	for _, n := range []string{"zscore", "standardize", "normalize", "cumsum"} {
		fields = append(fields, TableField{Key: n, Type: mapper()})
	}
	for _, n := range []string{"covariance", "correlation", "spearman", "weighted_mean"} {
		fields = append(fields, TableField{Key: n, Type: pair()})
	}
	fields = append(fields,
		TableField{Key: "percentile", Type: NewFunction([]*Type{anyT, numberT},
			[]*Type{numberT}, false, nil)},
		TableField{Key: "quantile", Type: NewFunction([]*Type{anyT, numberT},
			[]*Type{numberT}, false, nil)},
		TableField{Key: "describe", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
		TableField{Key: "histogram", Type: NewFunction([]*Type{anyT, Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		TableField{Key: "normal_pdf", Type: NewFunction(
			[]*Type{numberT, Optional(numberT), Optional(numberT)}, []*Type{numberT}, false, nil)},
		TableField{Key: "normal_cdf", Type: NewFunction(
			[]*Type{numberT, Optional(numberT), Optional(numberT)}, []*Type{numberT}, false, nil)},
		TableField{Key: "t_test_1sample", Type: NewFunction([]*Type{anyT, Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		TableField{Key: "t_test_2sample", Type: NewFunction([]*Type{anyT, anyT},
			[]*Type{anyT}, false, nil)},
	)
	return NewTable(fields, nil)
}

func linalgModuleType() *Type {
	mat := anyT
	mat2 := func() *Type { return NewFunction([]*Type{mat, mat}, []*Type{mat}, false, nil) }
	return NewTable([]TableField{
		{Key: "VERSION", Type: stringT},
		{Key: "add", Type: mat2()},
		{Key: "sub", Type: mat2()},
		{Key: "matmul", Type: mat2()},
		{Key: "matvec", Type: mat2()},
		{Key: "scale", Type: NewFunction([]*Type{mat, numberT}, []*Type{mat}, false, nil)},
		{Key: "transpose", Type: NewFunction([]*Type{mat}, []*Type{mat}, false, nil)},
		{Key: "identity", Type: NewFunction([]*Type{numberT}, []*Type{mat}, false, nil)},
		{Key: "zeros", Type: NewFunction([]*Type{numberT, Optional(numberT)}, []*Type{mat}, false, nil)},
		{Key: "ones", Type: NewFunction([]*Type{numberT, Optional(numberT)}, []*Type{mat}, false, nil)},
		{Key: "det", Type: NewFunction([]*Type{mat}, []*Type{numberT}, false, nil)},
		{Key: "trace", Type: NewFunction([]*Type{mat}, []*Type{numberT}, false, nil)},
		{Key: "norm", Type: NewFunction([]*Type{mat}, []*Type{numberT}, false, nil)},
		{Key: "dot", Type: NewFunction([]*Type{mat, mat}, []*Type{numberT}, false, nil)},
		{Key: "distance", Type: NewFunction([]*Type{mat, mat}, []*Type{numberT}, false, nil)},
		{Key: "inverse", Type: NewFunction([]*Type{mat}, []*Type{Optional(mat), Optional(anyT)}, false, nil)},
		{Key: "solve", Type: NewFunction([]*Type{mat, mat}, []*Type{Optional(mat), Optional(anyT)}, false, nil)},

		{Key: "cholesky", Type: NewFunction([]*Type{mat}, []*Type{mat}, false, nil)},
		{Key: "qr", Type: NewFunction([]*Type{mat}, []*Type{mat, mat}, false, nil)},
		{Key: "eigh", Type: NewFunction([]*Type{mat}, []*Type{mat, mat}, false, nil)},
		{Key: "lstsq", Type: NewFunction([]*Type{mat, mat}, []*Type{mat}, false, nil)},
		{Key: "rank", Type: NewFunction([]*Type{mat}, []*Type{numberT}, false, nil)},
	}, nil)
}

func testModuleType() *Type {
	assertFn := func() *Type { return NewFunction(nil, nil, true, anyT) }
	fields := []TableField{
		{Key: "VERSION", Type: stringT},
		{Key: "describe", Type: NewFunction([]*Type{stringT, anyT}, nil, false, nil)},
		{Key: "it", Type: NewFunction([]*Type{stringT, anyT}, nil, false, nil)},
		{Key: "test", Type: NewFunction([]*Type{stringT, anyT}, nil, false, nil)},
		{Key: "skip", Type: NewFunction([]*Type{stringT, Optional(anyT)}, nil, false, nil)},
		{Key: "fail", Type: NewFunction([]*Type{Optional(stringT)}, nil, false, nil)},
		{Key: "before_each", Type: NewFunction([]*Type{anyT}, nil, false, nil)},
		{Key: "after_each", Type: NewFunction([]*Type{anyT}, nil, false, nil)},
	}
	for _, n := range []string{
		"assert_eq", "assert_ne", "assert_true", "assert_false", "assert_nil",
		"assert_not_nil", "assert_error", "assert_no_error", "assert_type",
		"assert_match", "assert_contains", "assert_len", "assert_near",
		"assert_deep_eq",
	} {
		fields = append(fields, TableField{Key: n, Type: assertFn()})
	}
	return NewTable(fields, nil)
}

func nativeMathModuleType() *Type {
	fields := []TableField{{Key: "VERSION", Type: stringT}}
	for _, n := range []string{
		"pi", "e", "phi", "nan", "huge", "ln2", "ln10", "log2e", "log10e",
		"sqrt2", "sqrte", "sqrtpi", "sqrtphi",
		"maxfloat32", "maxfloat64", "smallestnonzerofloat32", "smallestnonzerofloat64",
		"maxint", "minint", "maxint8", "minint8", "maxint16", "minint16",
		"maxint32", "minint32", "maxint64", "minint64",
		"maxuint8", "maxuint16", "maxuint32",
	} {
		fields = append(fields, TableField{Key: n, Type: numberT})
	}
	for _, n := range []string{
		"abs", "acos", "acosh", "asin", "asinh", "atanh", "cbrt", "ceil",
		"cos", "cosh", "deg", "erf", "erfc", "exp", "floor", "rad",
		"sin", "sinh", "sqrt", "tan", "tanh",
	} {
		fields = append(fields, TableField{Key: n, Type: numToNum()})
	}
	for _, n := range []string{"fmod", "pow"} {
		fields = append(fields, TableField{Key: n, Type: num2ToNum()})
	}
	fields = append(fields,
		TableField{Key: "atan", Type: NewFunction([]*Type{numberT, Optional(numberT)},
			[]*Type{numberT}, false, nil)},
		TableField{Key: "log", Type: NewFunction([]*Type{numberT, Optional(numberT)},
			[]*Type{numberT}, false, nil)},
		TableField{Key: "clamp", Type: NewFunction([]*Type{numberT, numberT, numberT},
			[]*Type{numberT}, false, nil)},
		TableField{Key: "max", Type: NewFunction([]*Type{numberT}, []*Type{numberT}, true, numberT)},
		TableField{Key: "min", Type: NewFunction([]*Type{numberT}, []*Type{numberT}, true, numberT)},
		TableField{Key: "modf", Type: NewFunction([]*Type{numberT},
			[]*Type{numberT, numberT}, false, nil)},
		TableField{Key: "random", Type: NewFunction(nil, []*Type{numberT}, true, numberT)},
		TableField{Key: "tointeger", Type: NewFunction([]*Type{anyT},
			[]*Type{Optional(numberT)}, false, nil)},
		TableField{Key: "ult", Type: NewFunction([]*Type{numberT, numberT},
			[]*Type{booleanT}, false, nil)},
		TableField{Key: "mean", Type: NewFunction([]*Type{anyT}, []*Type{numberT}, false, nil)},
		TableField{Key: "variance", Type: NewFunction([]*Type{anyT}, []*Type{numberT}, false, nil)},
		TableField{Key: "standard_deviation", Type: NewFunction([]*Type{anyT},
			[]*Type{numberT}, false, nil)},
		TableField{Key: "softmax", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
	)
	return NewTable(fields, nil)
}

func strToStr() *Type {
	return NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)
}

func numToNum() *Type {
	return NewFunction([]*Type{numberT}, []*Type{numberT}, false, nil)
}

func num2ToNum() *Type {
	return NewFunction([]*Type{numberT, numberT}, []*Type{numberT}, false, nil)
}

func sortFn() *Type {
	return NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{anyT}, false, nil)
}

func logFn() *Type {
	return NewFunction(nil, nil, true, anyT)
}

func dateTableType() *Type {
	return NewTable([]TableField{
		{Key: "year", Type: numberT},
		{Key: "month", Type: numberT},
		{Key: "day", Type: numberT},
		{Key: "hour", Type: numberT},
		{Key: "min", Type: numberT},
		{Key: "sec", Type: numberT},
		{Key: "wday", Type: numberT},
		{Key: "yday", Type: numberT},
		{Key: "isdst", Type: booleanT},
	}, nil)
}

func regexObjectType() *Type {
	self := anyT
	return NewTable([]TableField{
		{Key: "test", Type: NewFunction([]*Type{self, stringT}, []*Type{booleanT}, false, nil)},
		{Key: "capture", Type: NewFunction([]*Type{self, stringT},
			[]*Type{Optional(stringT)}, true, stringT)},
		{Key: "find", Type: NewFunction([]*Type{self, stringT, Optional(numberT)},
			[]*Type{Optional(numberT), Optional(numberT)}, true, anyT)},
		{Key: "groups", Type: NewFunction([]*Type{self, stringT}, []*Type{Optional(anyT)}, false, nil)},
		{Key: "find_all", Type: NewFunction([]*Type{self, stringT, Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		{Key: "find_all_captures", Type: NewFunction([]*Type{self, stringT, Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		{Key: "replace", Type: NewFunction([]*Type{self, stringT, stringT}, []*Type{stringT}, false, nil)},
		{Key: "replace_func", Type: NewFunction([]*Type{self, stringT, anyT},
			[]*Type{stringT}, false, nil)},
		{Key: "split", Type: NewFunction([]*Type{self, stringT}, []*Type{anyT}, false, nil)},
	}, nil)
}

func httpResponseType() *Type {
	return NewTable([]TableField{
		{Key: "status", Type: numberT},
		{Key: "status_text", Type: stringT},
		{Key: "body", Type: stringT},
		{Key: "ok", Type: booleanT},
		{Key: "headers", Type: anyT},
		{Key: "headers_raw", Type: anyT},
		{Key: "url", Type: stringT},
	}, nil)
}

func httpModuleType() *Type {
	resp := []*Type{httpResponseType()}
	noBody := func() *Type {
		return NewFunction([]*Type{stringT, Optional(anyT)}, resp, false, nil)
	}
	withBody := func() *Type {
		return NewFunction([]*Type{stringT, Optional(stringT), Optional(anyT)}, resp, false, nil)
	}
	return NewTable([]TableField{
		{Key: "VERSION", Type: stringT},
		{Key: "MethodGet", Type: stringT},
		{Key: "MethodPost", Type: stringT},
		{Key: "MethodPut", Type: stringT},
		{Key: "MethodPatch", Type: stringT},
		{Key: "MethodDelete", Type: stringT},
		{Key: "MethodHead", Type: stringT},
		{Key: "MethodOptions", Type: stringT},
		{Key: "MethodTrace", Type: stringT},
		{Key: "get", Type: noBody()},
		{Key: "delete", Type: noBody()},
		{Key: "head", Type: noBody()},
		{Key: "options", Type: noBody()},
		{Key: "post", Type: withBody()},
		{Key: "put", Type: withBody()},
		{Key: "patch", Type: withBody()},
		{Key: "request", Type: NewFunction([]*Type{anyT}, resp, false, nil)},
		{Key: "new_client", Type: NewFunction([]*Type{Optional(anyT)}, []*Type{anyT}, false, nil)},
		{Key: "encode_url", Type: NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)},
	}, nil)
}

func osModuleType() *Type {
	fsResult := []*Type{anyT, Optional(stringT)}
	return NewTable([]TableField{
		{Key: "VERSION", Type: stringT},

		{Key: "time", Type: NewFunction([]*Type{Optional(anyT)}, []*Type{numberT}, false, nil)},
		{Key: "clock", Type: NewFunction(nil, []*Type{numberT}, false, nil)},
		{Key: "date", Type: NewFunction([]*Type{Optional(stringT), Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		{Key: "difftime", Type: num2ToNum()},

		{Key: "getenv", Type: NewFunction([]*Type{stringT}, []*Type{Optional(stringT)}, false, nil)},
		{Key: "setenv", Type: NewFunction([]*Type{stringT, stringT}, fsResult, false, nil)},
		{Key: "exit", Type: NewFunction([]*Type{Optional(anyT)}, nil, false, nil)},
		{Key: "execute", Type: NewFunction([]*Type{Optional(stringT)}, []*Type{anyT}, true, anyT)},
		{Key: "setlocale", Type: NewFunction([]*Type{Optional(stringT), Optional(stringT)},
			[]*Type{Optional(stringT)}, false, nil)},
		{Key: "hostname", Type: NewFunction(nil, []*Type{anyT, Optional(stringT)}, false, nil)},
		{Key: "platform", Type: stringT},
		{Key: "arch", Type: stringT},

		{Key: "pwd", Type: NewFunction(nil, []*Type{stringT}, false, nil)},
		{Key: "getcwd", Type: NewFunction(nil, []*Type{stringT}, false, nil)},
		{Key: "tmpname", Type: NewFunction(nil, []*Type{stringT}, false, nil)},
		{Key: "path_separator", Type: stringT},
		{Key: "path_list_separator", Type: stringT},
		{Key: "dev_null", Type: stringT},

		{Key: "remove", Type: NewFunction([]*Type{stringT}, fsResult, false, nil)},
		{Key: "rename", Type: NewFunction([]*Type{stringT, stringT}, fsResult, false, nil)},
		{Key: "mkdir", Type: NewFunction([]*Type{stringT, Optional(anyT)}, fsResult, false, nil)},
		{Key: "open", Type: NewFunction([]*Type{stringT, Optional(numberT), Optional(numberT)},
			[]*Type{anyT, Optional(stringT)}, false, nil)},
		{Key: "create", Type: NewFunction([]*Type{stringT},
			[]*Type{anyT, Optional(stringT)}, false, nil)},

		{Key: "o_rdonly", Type: numberT},
		{Key: "o_wronly", Type: numberT},
		{Key: "o_rdwr", Type: numberT},
		{Key: "o_append", Type: numberT},
		{Key: "o_create", Type: numberT},
		{Key: "o_excl", Type: numberT},
		{Key: "o_sync", Type: numberT},
		{Key: "o_trunc", Type: numberT},
		{Key: "seek_set", Type: numberT},
		{Key: "seek_cur", Type: numberT},
		{Key: "seek_end", Type: numberT},

		{Key: "mode_dir", Type: numberT},
		{Key: "mode_perm", Type: numberT},
		{Key: "mode_type", Type: numberT},
		{Key: "mode_append", Type: numberT},
		{Key: "mode_exclusive", Type: numberT},
		{Key: "mode_temporary", Type: numberT},
		{Key: "mode_symlink", Type: numberT},
		{Key: "mode_device", Type: numberT},
		{Key: "mode_named_pipe", Type: numberT},
		{Key: "mode_socket", Type: numberT},
		{Key: "mode_setuid", Type: numberT},
		{Key: "mode_setgid", Type: numberT},
		{Key: "mode_char_device", Type: numberT},
		{Key: "mode_sticky", Type: numberT},
	}, nil)
}
