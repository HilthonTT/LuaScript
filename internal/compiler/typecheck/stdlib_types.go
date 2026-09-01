package typecheck

func stdlibGlobals() map[string]*Type {
	g := map[string]*Type{}

	g["print"] = NewFunction(nil, nil, true, anyT)

	g["type"] = NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)

	g["typeof"] = NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)

	g["sizeof"] = NewFunction([]*Type{anyT}, []*Type{numberT}, false, nil)

	g["collectgarbage"] = NewFunction([]*Type{Optional(stringT), Optional(numberT)}, []*Type{anyT}, false, nil)

	g["tostring"] = NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)

	numberOrNil := Optional(numberT)
	g["tonumber"] = NewFunction([]*Type{anyT, Optional(numberT)}, []*Type{numberOrNil}, false, nil)

	iter3 := []*Type{anyT, anyT, anyT}
	g["pairs"] = NewFunction([]*Type{anyT}, iter3, false, nil)
	g["ipairs"] = NewFunction([]*Type{anyT}, iter3, false, nil)
	g["next"] = NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{anyT, anyT}, false, nil)

	g["error"] = NewFunction([]*Type{anyT, Optional(numberT)}, []*Type{neverT}, false, nil)

	g["pcall"] = NewFunction([]*Type{anyT}, []*Type{booleanT, anyT}, true, anyT)

	g["xpcall"] = NewFunction([]*Type{anyT, anyT}, []*Type{booleanT, anyT}, true, anyT)

	g["assert"] = NewFunction([]*Type{anyT, Optional(stringT)}, []*Type{anyT}, false, nil)

	g["select"] = NewFunction([]*Type{anyT}, []*Type{anyT}, true, anyT)

	g["setmetatable"] = NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{anyT}, false, nil)
	g["getmetatable"] = NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)
	g["rawget"] = NewFunction([]*Type{anyT, anyT}, []*Type{anyT}, false, nil)
	g["rawset"] = NewFunction([]*Type{anyT, anyT, anyT}, []*Type{anyT}, false, nil)
	g["rawequal"] = NewFunction([]*Type{anyT, anyT}, []*Type{booleanT}, false, nil)
	g["rawlen"] = NewFunction([]*Type{anyT}, []*Type{numberT}, false, nil)

	g["require"] = NewFunction([]*Type{stringT}, []*Type{anyT}, false, nil)
	g["loadfile"] = NewFunction([]*Type{stringT, Optional(stringT), Optional(anyT)},
		[]*Type{Optional(NewFunction(nil, []*Type{anyT}, true, anyT)), Optional(stringT)}, false, nil)
	g["dofile"] = NewFunction([]*Type{Optional(stringT)}, []*Type{anyT}, true, anyT)
	g["load"] = NewFunction([]*Type{anyT, Optional(stringT), Optional(stringT), Optional(anyT)},
		[]*Type{Optional(anyT), Optional(stringT)}, false, nil)

	g["_VERSION"] = stringT
	g["_G"] = anyT

	g["math"] = mathModule()
	g["string"] = stringModule()
	g["table"] = tableModule()
	g["io"] = ioModule()
	g["coroutine"] = coroutineModule()
	g["package"] = packageModule()

	return g
}

func mathModule() *Type {
	one := []*Type{numberT}
	twoToOne := NewFunction([]*Type{numberT, numberT}, one, false, nil)
	oneToOne := NewFunction([]*Type{numberT}, one, false, nil)

	return NewTable([]TableField{
		{Key: "pi", Type: numberT},
		{Key: "huge", Type: numberT},
		{Key: "maxinteger", Type: numberT},
		{Key: "mininteger", Type: numberT},

		{Key: "abs", Type: oneToOne},
		{Key: "ceil", Type: oneToOne},
		{Key: "floor", Type: oneToOne},
		{Key: "sqrt", Type: oneToOne},
		{Key: "exp", Type: oneToOne},
		{Key: "sin", Type: oneToOne},
		{Key: "cos", Type: oneToOne},
		{Key: "tan", Type: oneToOne},
		{Key: "asin", Type: oneToOne},
		{Key: "acos", Type: oneToOne},

		{Key: "atan", Type: NewFunction([]*Type{numberT, Optional(numberT)}, one, false, nil)},
		{Key: "log", Type: NewFunction([]*Type{numberT, Optional(numberT)}, one, false, nil)},

		{Key: "fmod", Type: twoToOne},
		{Key: "pow", Type: twoToOne},

		{Key: "modf", Type: NewFunction([]*Type{numberT}, []*Type{numberT, numberT}, false, nil)},

		{Key: "max", Type: NewFunction([]*Type{numberT}, one, true, numberT)},
		{Key: "min", Type: NewFunction([]*Type{numberT}, one, true, numberT)},

		{Key: "ult", Type: NewFunction([]*Type{numberT, numberT}, []*Type{booleanT}, false, nil)},

		{Key: "tointeger", Type: NewFunction([]*Type{anyT}, []*Type{Optional(numberT)}, false, nil)},

		{Key: "type", Type: NewFunction([]*Type{anyT}, []*Type{Optional(stringT)}, false, nil)},

		{Key: "random", Type: NewFunction(nil, one, true, numberT)},
		{Key: "randomseed", Type: NewFunction([]*Type{Optional(numberT)},
			[]*Type{numberT, numberT}, false, nil)},
	}, nil)
}

func stringModule() *Type {
	return NewTable([]TableField{
		{Key: "len", Type: NewFunction([]*Type{stringT}, []*Type{numberT}, false, nil)},

		{Key: "upper", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)},
		{Key: "lower", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)},
		{Key: "reverse", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)},

		{Key: "rep", Type: NewFunction([]*Type{stringT, numberT, Optional(stringT)},
			[]*Type{stringT}, false, nil)},

		{Key: "sub", Type: NewFunction([]*Type{stringT, numberT, Optional(numberT)},
			[]*Type{stringT}, false, nil)},

		{Key: "byte", Type: NewFunction([]*Type{stringT, Optional(numberT), Optional(numberT)},
			[]*Type{Optional(numberT)}, true, numberT)},

		{Key: "char", Type: NewFunction(nil, []*Type{stringT}, true, numberT)},

		{Key: "find", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(numberT), Optional(booleanT)},
			[]*Type{Optional(numberT), Optional(numberT)}, false, nil)},

		{Key: "format", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, true, anyT)},

		{Key: "pack", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, true, anyT)},

		{Key: "unpack", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(numberT)},
			[]*Type{anyT}, true, anyT)},

		{Key: "packsize", Type: NewFunction([]*Type{stringT}, []*Type{numberT}, false, nil)},

		{Key: "match", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(numberT)},
			[]*Type{Optional(anyT)}, true, anyT)},
		{Key: "gmatch", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(numberT)},
			[]*Type{anyT}, false, nil)},
		{Key: "gsub", Type: NewFunction(
			[]*Type{stringT, stringT, anyT, Optional(numberT)},
			[]*Type{stringT, numberT}, false, nil)},
	}, nil)
}

func tableModule() *Type {
	return NewTable([]TableField{
		{Key: "insert", Type: NewFunction([]*Type{anyT, anyT, Optional(anyT)}, nil, false, nil)},

		{Key: "remove", Type: NewFunction([]*Type{anyT, Optional(numberT)},
			[]*Type{Optional(anyT)}, false, nil)},

		{Key: "concat", Type: NewFunction(
			[]*Type{anyT, Optional(stringT), Optional(numberT), Optional(numberT)},
			[]*Type{stringT}, false, nil)},

		{Key: "sort", Type: NewFunction(
			[]*Type{anyT, Optional(NewFunction([]*Type{anyT, anyT}, []*Type{booleanT}, false, nil))},
			nil, false, nil)},

		{Key: "move", Type: NewFunction(
			[]*Type{anyT, numberT, numberT, numberT, Optional(anyT)},
			[]*Type{anyT}, false, nil)},

		{Key: "unpack", Type: NewFunction([]*Type{anyT, Optional(numberT), Optional(numberT)},
			[]*Type{anyT}, true, anyT)},

		{Key: "pack", Type: NewFunction(nil,
			[]*Type{NewTable(
				[]TableField{{Key: "n", Type: numberT}},
				&Indexer{Key: numberT, Value: anyT},
			)}, true, anyT)},
	}, nil)
}

func fileHandleType() *Type {
	self := anyT
	return NewTable([]TableField{
		{Key: "read", Type: NewFunction([]*Type{self}, []*Type{Optional(anyT)}, true, anyT)},
		{Key: "write", Type: NewFunction([]*Type{self}, []*Type{anyT, Optional(stringT)}, true, anyT)},
		{Key: "lines", Type: NewFunction([]*Type{self}, []*Type{anyT}, true, anyT)},
		{Key: "seek", Type: NewFunction([]*Type{self, Optional(stringT), Optional(numberT)},
			[]*Type{Optional(numberT), Optional(stringT)}, false, nil)},
		{Key: "flush", Type: NewFunction([]*Type{self}, []*Type{anyT, Optional(stringT)}, false, nil)},
		{Key: "close", Type: NewFunction([]*Type{self}, []*Type{anyT, Optional(stringT)}, false, nil)},
	}, nil)
}

func ioModule() *Type {
	file := fileHandleType()
	openResult := []*Type{Optional(file), Optional(stringT)}

	return NewTable([]TableField{
		{Key: "open", Type: NewFunction([]*Type{stringT, Optional(stringT)}, openResult, false, nil)},
		{Key: "lines", Type: NewFunction([]*Type{Optional(stringT)}, []*Type{anyT}, true, anyT)},
		{Key: "read", Type: NewFunction(nil, []*Type{Optional(anyT)}, true, anyT)},
		{Key: "write", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "close", Type: NewFunction([]*Type{Optional(anyT)},
			[]*Type{anyT, Optional(stringT)}, false, nil)},
		{Key: "flush", Type: NewFunction(nil, []*Type{anyT}, false, nil)},
		{Key: "tmpfile", Type: NewFunction(nil, openResult, false, nil)},
		{Key: "type", Type: NewFunction([]*Type{anyT}, []*Type{Optional(stringT)}, false, nil)},
		{Key: "input", Type: NewFunction([]*Type{Optional(anyT)}, []*Type{file}, false, nil)},
		{Key: "output", Type: NewFunction([]*Type{Optional(anyT)}, []*Type{file}, false, nil)},

		{Key: "stdin", Type: file},
		{Key: "stdout", Type: file},
		{Key: "stderr", Type: file},
	}, nil)
}

func coroutineModule() *Type {
	return NewTable([]TableField{
		{Key: "create", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
		{Key: "resume", Type: NewFunction([]*Type{anyT}, []*Type{booleanT, anyT}, true, anyT)},
		{Key: "yield", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		{Key: "status", Type: NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)},
		{Key: "wrap", Type: NewFunction([]*Type{anyT},
			[]*Type{NewFunction(nil, []*Type{anyT}, true, anyT)}, false, nil)},
		{Key: "isyieldable", Type: NewFunction(nil, []*Type{booleanT}, false, nil)},
		{Key: "running", Type: NewFunction(nil, []*Type{anyT, booleanT}, false, nil)},
		{Key: "close", Type: NewFunction([]*Type{anyT}, []*Type{booleanT, anyT}, false, nil)},
	}, nil)
}

func packageModule() *Type {
	return NewTable([]TableField{
		{Key: "path", Type: stringT},
		{Key: "config", Type: stringT},
		{Key: "loaded", Type: NewTable(nil, &Indexer{Key: stringT, Value: anyT})},
		{Key: "preload", Type: NewTable(nil, &Indexer{Key: stringT, Value: anyT})},
		{Key: "searchpath", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(stringT), Optional(stringT)},
			[]*Type{Optional(stringT)}, false, nil)},
	}, nil)
}
