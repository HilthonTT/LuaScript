package typecheck

// stdlib_types.go declares the type signatures of every built-in name
// reachable from.lsc programs. The list mirrors the VM's stdlib
// registration files (vm/stdlib.go, vm/stdlib_modules.go, vm/coroutine.go,
// vm/loader.go) — additions there must be reflected here, otherwise the
// checker treats new globals as `any` (gradual fallback) which silently
// loses type safety.
//
// Signatures match Lua 5.4 documentation, narrowed to what each Go closure
// actually accepts. Where the implementation only supports a subset (e.g.
// `string.find`'s `plain` mode is the only mode), the signature reflects
// the supported surface, not the full Lua 5.4 spec.

// stdlibGlobals returns a map of every top-level name to its type. The
// checker installs these into the env's outermost frame on Check().
func stdlibGlobals() map[string]*Type {
	g := map[string]*Type{}

	// ----- top-level globals (vm/stdlib.go) -----------------------------------

	// print(...) -> ()
	g["print"] = NewFunction(nil, nil, true, anyT)

	// type(any) -> string
	g["type"] = NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)

	// tostring(any) -> string
	g["tostring"] = NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)

	// tonumber(any [, base]) -> number | nil
	numberOrNil := Optional(numberT)
	g["tonumber"] = NewFunction([]*Type{anyT, Optional(numberT)}, []*Type{numberOrNil}, false, nil)

	// pairs / ipairs / next return iterator triples — modeled loosely as
	// `(any) -> (any, any, any)` so generic-for loops type-check without
	// needing iterator-protocol modeling.
	iter3 := []*Type{anyT, anyT, anyT}
	g["pairs"] = NewFunction([]*Type{anyT}, iter3, false, nil)
	g["ipairs"] = NewFunction([]*Type{anyT}, iter3, false, nil)
	g["next"] = NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{anyT, anyT}, false, nil)

	// error(any [, level]) -> never
	g["error"] = NewFunction([]*Type{anyT, Optional(numberT)}, []*Type{neverT}, false, nil)

	// pcall(f, ...) -> (boolean, ...) — returns success flag plus either
	// the call's results or its error.
	g["pcall"] = NewFunction([]*Type{anyT}, []*Type{booleanT, anyT}, true, anyT)

	// assert(v, [msg]) -> (v) — passes through truthy values; we model
	// the result as `any` because untyped flow is the common case.
	g["assert"] = NewFunction([]*Type{anyT, Optional(stringT)}, []*Type{anyT}, false, nil)

	// select(idx_or_'#', ...) -> any...
	g["select"] = NewFunction([]*Type{anyT}, []*Type{anyT}, true, anyT)

	// setmetatable(t, mt) -> t
	g["setmetatable"] = NewFunction([]*Type{anyT, Optional(anyT)}, []*Type{anyT}, false, nil)
	g["getmetatable"] = NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)
	g["rawget"] = NewFunction([]*Type{anyT, anyT}, []*Type{anyT}, false, nil)
	g["rawset"] = NewFunction([]*Type{anyT, anyT, anyT}, []*Type{anyT}, false, nil)
	g["rawequal"] = NewFunction([]*Type{anyT, anyT}, []*Type{booleanT}, false, nil)
	g["rawlen"] = NewFunction([]*Type{anyT}, []*Type{numberT}, false, nil)

	// ----- loader (vm/loader.go) ----------------------------------------------

	// require(modname) -> any — cross-module typing is a v2 problem;
	// `require` returns `any` so call sites flow loosely.
	g["require"] = NewFunction([]*Type{stringT}, []*Type{anyT}, false, nil)
	g["loadfile"] = NewFunction([]*Type{stringT, Optional(stringT), Optional(anyT)},
		[]*Type{Optional(NewFunction(nil, []*Type{anyT}, true, anyT)), Optional(stringT)}, false, nil)
	g["dofile"] = NewFunction([]*Type{Optional(stringT)}, []*Type{anyT}, true, anyT)
	g["load"] = NewFunction([]*Type{anyT, Optional(stringT), Optional(stringT), Optional(anyT)},
		[]*Type{Optional(anyT), Optional(stringT)}, false, nil)

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
		// constants
		{Key: "pi", Type: numberT},
		{Key: "huge", Type: numberT},
		{Key: "maxinteger", Type: numberT},
		{Key: "mininteger", Type: numberT},

		// (number) -> number
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

		// (number [, number]) -> number
		{Key: "atan", Type: NewFunction([]*Type{numberT, Optional(numberT)}, one, false, nil)},
		{Key: "log", Type: NewFunction([]*Type{numberT, Optional(numberT)}, one, false, nil)},

		// (number, number) -> number
		{Key: "fmod", Type: twoToOne},
		{Key: "pow", Type: twoToOne},

		// modf(n) -> (integer-part, fractional-part)
		{Key: "modf", Type: NewFunction([]*Type{numberT}, []*Type{numberT, numberT}, false, nil)},

		// max/min(...) -> number
		{Key: "max", Type: NewFunction([]*Type{numberT}, one, true, numberT)},
		{Key: "min", Type: NewFunction([]*Type{numberT}, one, true, numberT)},

		// tointeger(any) -> number | nil
		{Key: "tointeger", Type: NewFunction([]*Type{anyT}, []*Type{Optional(numberT)}, false, nil)},

		// type(any) -> "integer" | "float" | nil — modeled as string|nil.
		{Key: "type", Type: NewFunction([]*Type{anyT}, []*Type{Optional(stringT)}, false, nil)},

		// random([m [, n]]) -> number
		{Key: "random", Type: NewFunction(nil, one, true, numberT)},
		{Key: "randomseed", Type: NewFunction([]*Type{Optional(numberT)},
			[]*Type{numberT, numberT}, false, nil)},
	}, nil)
}

func stringModule() *Type {
	return NewTable([]TableField{
		// (string) -> number
		{Key: "len", Type: NewFunction([]*Type{stringT}, []*Type{numberT}, false, nil)},

		// (string) -> string
		{Key: "upper", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)},
		{Key: "lower", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)},
		{Key: "reverse", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, false, nil)},

		// (string, number) -> string
		{Key: "rep", Type: NewFunction([]*Type{stringT, numberT, Optional(stringT)},
			[]*Type{stringT}, false, nil)},

		// (string, number [, number]) -> string
		{Key: "sub", Type: NewFunction([]*Type{stringT, numberT, Optional(numberT)},
			[]*Type{stringT}, false, nil)},

		// (string, number [, number]) -> ...number
		{Key: "byte", Type: NewFunction([]*Type{stringT, Optional(numberT), Optional(numberT)},
			[]*Type{numberT}, true, numberT)},

		// (...number) -> string
		{Key: "char", Type: NewFunction(nil, []*Type{stringT}, true, numberT)},

		// (string, string [, init [, plain]]) -> (number, number) | nil
		// Implementation supports plain-only, but the signature accepts
		// the full positional shape so existing call sites still type.
		{Key: "find", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(numberT), Optional(booleanT)},
			[]*Type{Optional(numberT), Optional(numberT)}, false, nil)},

		// (string, ...) -> string
		{Key: "format", Type: NewFunction([]*Type{stringT}, []*Type{stringT}, true, anyT)},

		// Lua-pattern members. Captures are dynamic — typed as any — so
		// match/gmatch results compose with the rest of the runtime.
		{Key: "match", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(numberT)},
			[]*Type{Optional(anyT)}, true, anyT)},
		{Key: "gmatch", Type: NewFunction(
			[]*Type{stringT, stringT},
			[]*Type{anyT}, false, nil)},
		{Key: "gsub", Type: NewFunction(
			[]*Type{stringT, stringT, anyT, Optional(numberT)},
			[]*Type{stringT, numberT}, false, nil)},
	}, nil)
}

func tableModule() *Type {
	return NewTable([]TableField{
		// table.insert(t, [pos,] v) — model as (any, any[, any]) -> () with
		// vararg to absorb the optional positional argument.
		{Key: "insert", Type: NewFunction([]*Type{anyT, anyT, Optional(anyT)}, nil, false, nil)},

		// table.remove(t [, pos]) -> any | nil
		{Key: "remove", Type: NewFunction([]*Type{anyT, Optional(numberT)},
			[]*Type{Optional(anyT)}, false, nil)},

		// table.concat(t [, sep [, i [, j]]]) -> string
		{Key: "concat", Type: NewFunction(
			[]*Type{anyT, Optional(stringT), Optional(numberT), Optional(numberT)},
			[]*Type{stringT}, false, nil)},

		// table.unpack(t [, i [, j]]) -> ...any
		{Key: "unpack", Type: NewFunction([]*Type{anyT, Optional(numberT), Optional(numberT)},
			[]*Type{anyT}, true, anyT)},

		// table.pack(...) -> { n: number, [number]: any }
		{Key: "pack", Type: NewFunction(nil,
			[]*Type{NewTable(
				[]TableField{{Key: "n", Type: numberT}},
				&Indexer{Key: numberT, Value: anyT},
			)}, true, anyT)},
	}, nil)
}

func ioModule() *Type {
	return NewTable([]TableField{
		{Key: "write", Type: NewFunction(nil, nil, true, anyT)},
		// read takes a format string ("l" or "L"); we model it as
		// (string) -> string|nil.
		{Key: "read", Type: NewFunction([]*Type{Optional(stringT)},
			[]*Type{Optional(stringT)}, false, nil)},
	}, nil)
}

func coroutineModule() *Type {
	return NewTable([]TableField{
		// create(fn) -> thread (modeled as `any`)
		{Key: "create", Type: NewFunction([]*Type{anyT}, []*Type{anyT}, false, nil)},
		// resume(co, ...) -> (boolean, ...)
		{Key: "resume", Type: NewFunction([]*Type{anyT}, []*Type{booleanT, anyT}, true, anyT)},
		// yield(...) -> ...
		{Key: "yield", Type: NewFunction(nil, []*Type{anyT}, true, anyT)},
		// status(co) -> string
		{Key: "status", Type: NewFunction([]*Type{anyT}, []*Type{stringT}, false, nil)},
		// wrap(fn) -> function
		{Key: "wrap", Type: NewFunction([]*Type{anyT},
			[]*Type{NewFunction(nil, []*Type{anyT}, true, anyT)}, false, nil)},
		// isyieldable() -> boolean
		{Key: "isyieldable", Type: NewFunction(nil, []*Type{booleanT}, false, nil)},
	}, nil)
}

func packageModule() *Type {
	return NewTable([]TableField{
		{Key: "path", Type: stringT},
		{Key: "config", Type: stringT},
		{Key: "loaded", Type: NewTable(nil, &Indexer{Key: stringT, Value: anyT})},
		{Key: "preload", Type: NewTable(nil, &Indexer{Key: stringT, Value: anyT})},
		// searchpath(name, path [, sep [, rep]]) -> string | nil
		{Key: "searchpath", Type: NewFunction(
			[]*Type{stringT, stringT, Optional(stringT), Optional(stringT)},
			[]*Type{Optional(stringT)}, false, nil)},
	}, nil)
}
