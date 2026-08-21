package docs

// Base library and language syntax. These names are installed into _G by
// internal/vm/stdlib.go, internal/vm/loader.go and internal/vm/coroutine.go;
// they need no require.

var coreTopics = []Topic{
	{
		Name:          "_G",
		Kind:          KindCore,
		RuntimeGlobal: "_G",
		Aliases:       []string{"globals", "base"},
		Title:         "base library — globals available without a require",
		Synopsis:      `print("hello")   -- no require needed`,
		Detail: `Every name here is installed directly into the global table when the
VM starts. The auto-global namespaces (string, table, math, coroutine, io)
have pages of their own; see SEE ALSO.

Two of these are luascript extensions rather than Lua 5.4: typeof and
sizeof.`,
		SeeAlso: []string{"string", "table", "math", "coroutine", "io", "syntax"},
		Entries: []Entry{
			{Name: "print", Kind: EntryFunction, Signature: "print(...)",
				Summary: "Writes each argument to stdout separated by tabs, followed by a newline. Routes values through the __tostring metamethod."},
			{Name: "type", Kind: EntryFunction, Signature: `type(v): string`,
				Summary: `Returns the type name of v: "nil", "boolean", "number", "string", "table", "function", "thread" or "userdata".`},
			{Name: "typeof", Kind: EntryFunction, Signature: `typeof(v): string`,
				Summary: "luascript extension. Like type, but reports the richer runtime name of a value.",
				Detail:  "See examples/33_typeof_sizeof.lsc."},
			{Name: "sizeof", Kind: EntryFunction, Signature: "sizeof(v): number",
				Summary: "luascript extension. Returns the size of a value — the entry count for a table, the byte length for a string.",
				Detail:  "See examples/33_typeof_sizeof.lsc."},
			{Name: "tostring", Kind: EntryFunction, Signature: "tostring(v): string",
				Summary: "Converts v to a string, honouring the __tostring metamethod. Panics when __tostring returns a non-string."},
			{Name: "tonumber", Kind: EntryFunction, Signature: "tonumber(v [, base]): number?",
				Summary: "Converts v to a number, optionally reading it in the given base (2..36). Returns nil when the conversion fails."},
			{Name: "ipairs", Kind: EntryFunction, Signature: "ipairs(t): iterator",
				Summary: "Returns an iterator over the array part of t, walking 1..n and stopping at the first nil."},
			{Name: "pairs", Kind: EntryFunction, Signature: "pairs(t): iterator",
				Summary: "Returns an iterator over every key/value pair of t. Array entries come first, then hash entries in insertion order."},
			{Name: "next", Kind: EntryFunction, Signature: "next(t [, key]): any, any",
				Summary: "Returns the key/value pair that follows key in t, or nil when the traversal is finished. Passing nil starts it."},
			{Name: "error", Kind: EntryFunction, Signature: "error(message [, level])",
				Summary: "Raises message as an error, unwinding until a pcall, a try/catch, or the host catches it. The value propagates verbatim — it need not be a string."},
			{Name: "assert", Kind: EntryFunction, Signature: "assert(v [, message]): ...",
				Summary: "Raises an error when v is nil or false, otherwise returns every argument unchanged.",
				Detail:  "The typechecker narrows on assert: after `assert(x ~= nil)`, x is non-optional for the rest of the block."},
			{Name: "pcall", Kind: EntryFunction, Signature: "pcall(f, ...): boolean, ...",
				Summary: "Calls f in protected mode. Returns true plus f's results, or false plus the error value."},
			{Name: "xpcall", Kind: EntryFunction, Signature: "xpcall(f, msgh, ...): boolean, ...",
				Summary: "Like pcall, but routes a raised error through the handler msgh before returning it."},
			{Name: "select", Kind: EntryFunction, Signature: `select(n, ...): ...`,
				Summary: `Returns the arguments from index n onward, or their count when n is "#".`},
			{Name: "setmetatable", Kind: EntryFunction, Signature: "setmetatable(t, mt): table",
				Summary: "Sets t's metatable to mt (nil removes it) and returns t."},
			{Name: "getmetatable", Kind: EntryFunction, Signature: "getmetatable(t): table?",
				Summary: "Returns t's metatable, or nil when it has none."},
			{Name: "rawget", Kind: EntryFunction, Signature: "rawget(t, k): any",
				Summary: "Reads t[k] without consulting the __index metamethod."},
			{Name: "rawset", Kind: EntryFunction, Signature: "rawset(t, k, v): table",
				Summary: "Writes t[k] = v without consulting the __newindex metamethod, and returns t."},
			{Name: "rawequal", Kind: EntryFunction, Signature: "rawequal(a, b): boolean",
				Summary: "Compares a and b without consulting the __eq metamethod."},
			{Name: "rawlen", Kind: EntryFunction, Signature: "rawlen(v): number",
				Summary: "Returns the length of a table or string without consulting the __len metamethod."},
			{Name: "require", Kind: EntryFunction, Signature: "require(name): any",
				Summary: "Loads and returns a module, caching it in package.loaded.",
				Detail: `Resolution order: package.preload (where every native module lives),
then package.path — the directory of the running script first, then
cwd-relative entries, then $LUASCRIPT_LIB when it is set.`},
			{Name: "load", Kind: EntryFunction, Signature: "load(chunk [, chunkname]): function?, string?",
				Summary: "Compiles a source string into a function. Returns nil plus an error message when it does not compile.",
				Detail:  "Strings compiled through load are not written to the bytecode cache."},
			{Name: "loadfile", Kind: EntryFunction, Signature: "loadfile(path): function?, string?",
				Summary: "Compiles a file into a function without running it. Returns nil plus an error message on failure."},
			{Name: "dofile", Kind: EntryFunction, Signature: "dofile(path): ...",
				Summary: "Compiles and immediately runs a file, returning its results. Errors propagate to the caller."},
			{Name: "collectgarbage", Kind: EntryFunction, Signature: `collectgarbage([opt [, arg]]): any`,
				Summary: `Controls the host garbage collector. Options mirror the -gc-percent and -mem-limit CLI flags.`},
			{Name: "package", Kind: EntryField, Signature: "package: table",
				Summary: "The module system's state: package.path, package.loaded, package.preload, package.config and package.searchpath."},
			{Name: "_VERSION", Kind: EntryField, Signature: "_VERSION: string",
				Summary: `The language level this runtime implements — the string "Lua 5.4".`,
				Detail: `Reports the Lua level rather than a luascript version so that existing
scripts branching on _VERSION behave correctly. Use the -v CLI flag for
the interpreter's own version.`},
		},
	},
	{
		Name:     "syntax",
		Kind:     KindCore,
		Aliases:  []string{"keywords"},
		Title:    "reserved words and the constructs they introduce",
		Synopsis: `luascript doc syntax.match`,
		Detail: `luascript is Lua 5.4 syntax plus Luau-style gradual types and a handful
of extra statements. Several extensions are desugared in the parser and
have no dedicated bytecode: compound assignment, backtick string
interpolation, and match.

type, struct and continue are contextual keywords — they still work as
ordinary identifiers where no such statement can start. match, enum,
defer, try, catch and throw are hard keywords.`,
		SeeAlso: []string{"_G"},
		Entries: []Entry{
			{Name: "local", Kind: EntryKeyword, Signature: "local x [, y] = expr [, expr]",
				Summary: "Declares block-scoped variables. Attributes are supported: `local x <const> = 1` and `local h <close> = f()`."},
			{Name: "function", Kind: EntryKeyword, Signature: "function f(a: T, b: T = default): R ... end",
				Summary: "Declares a function. Parameters may carry type annotations and default values; a defaulted parameter falls back only on nil, not on false."},
			{Name: "if", Kind: EntryKeyword, Signature: "if c then ... elseif c2 then ... else ... end",
				Summary: "Conditional statement. There is also an if *expression*: `local v = if c then a else b` — no end, arms are single expressions, else is mandatory."},
			{Name: "for", Kind: EntryKeyword, Signature: "for i = a, b [, step] do ... end  |  for k, v in pairs(t) do ... end",
				Summary: "Numeric and generic loops. Both forms close upvalues on every exit path, including break and continue."},
			{Name: "while", Kind: EntryKeyword, Signature: "while cond do ... end",
				Summary: "Loops while cond is truthy."},
			{Name: "repeat", Kind: EntryKeyword, Signature: "repeat ... until cond",
				Summary: "Loops until cond is truthy; the body always runs once. A continue inside jumps to the until condition, so the condition may not read a local declared after that continue."},
			{Name: "break", Kind: EntryKeyword, Signature: "break",
				Summary: "Exits the innermost enclosing loop. Does not cross a function boundary."},
			{Name: "continue", Kind: EntryKeyword, Signature: "continue",
				Summary: "Skips to the next iteration of the innermost loop. A contextual keyword: `continue = 1` still parses as an identifier."},
			{Name: "goto", Kind: EntryKeyword, Signature: "goto label  ::label::",
				Summary: "Jumps to a label in the same function. Jumping across a try boundary is a compile error."},
			{Name: "return", Kind: EntryKeyword, Signature: "return [expr, ...]",
				Summary: "Returns from the enclosing function. Must be the last statement in its block."},
			{Name: "defer", Kind: EntryKeyword, Signature: "defer ... end",
				Summary: "Registers a block to run when the enclosing function returns, however it returns — including through an error. See examples/32_defer.lsc."},
			{Name: "try", Kind: EntryKeyword, Signature: "try <body> catch [e] do <handler> end",
				Summary: "Runs body with a protected region installed in the current frame.",
				Detail: `Because the region lives in the enclosing frame rather than in a
closure, return, break and continue inside a try act on the enclosing
function or loop, and the body's locals are ordinary frame slots. The
error binding is optional; the do is not. See examples/55_try_catch.lsc.`},
			{Name: "throw", Kind: EntryKeyword, Signature: "throw expr",
				Summary: "Raises expr as an error. A dedicated opcode, not a call to error, so shadowing error cannot re-point it."},
			{Name: "match", Kind: EntryKeyword, Signature: "match subject do pattern -> expr ... end",
				Summary: "Pattern-matching statement. A first-class node: the bytecode generator lowers it to one test-and-branch per arm.",
				Detail: `Destructuring patterns are gated on names declared in the chunk:
Circle(r) destructures only when Circle is a payload-carrying enum
variant, and Point{ x = a } only when Point is a struct. Any other
call-shaped pattern is compared by value.

When the subject's type has a finite domain — a tagged enum, a singleton
union, an enum, or boolean — the arms are checked for exhaustiveness and
a missing case is a compile error. Guarded arms do not count as coverage,
since a guard can fail. An untyped subject is never checked, so plain Lua
is unaffected. See examples/44_match.lsc.`},
			{Name: "enum", Kind: EntryKeyword, Signature: "enum Name V1, V2 end",
				Summary: "Declares an enum. Lowered to a frozen table of auto-incrementing integers; the typechecker types the alias as the exact literal union of those values (RED, GREEN => 1 | 2), so out-of-range numbers are rejected and match can check exhaustiveness. Payload-carrying (tagged) variants are supported — see examples/43_tagged_enums.lsc."},
			{Name: "struct", Kind: EntryKeyword, Signature: "struct Name x: number, y: number end",
				Summary: "Declares a struct type and its constructor. A contextual keyword. See examples/42_structs.lsc."},
			{Name: "type", Kind: EntryKeyword, Signature: "type Name = T  |  type Box<T> = { value: T }",
				Summary: "Declares a type alias, optionally generic. A contextual keyword. Aliases may be primitives, literals, unions, optionals, function types or structural tables."},
			{Name: "literal types", Kind: EntryKeyword, Signature: `type Mode = "read" | "write" | "append"`,
				Summary: "A string, number or boolean literal is a type of its own, inhabited by that one value.",
				Detail: `A singleton is assignable to its base primitive but not the reverse, so
a slot annotated "read" | "write" rejects every other string. Comparing
against one narrows a union (if m == "read" then ...), and a union of
singletons is what gives match a domain to check exhaustively.

Inference widens: local m = "read" is a string, so it stays assignable.
Write the annotation (local m: Mode = "read") or assert ("read" :: Mode)
to keep the singleton. See examples/05_types.lsc.`},
			{Name: "and", Kind: EntryKeyword, Signature: "a and b",
				Summary: "Short-circuit conjunction: returns a when a is falsy, otherwise b. The typechecker narrows across it."},
			{Name: "or", Kind: EntryKeyword, Signature: "a or b",
				Summary: "Short-circuit disjunction: returns a when a is truthy, otherwise b. `x or default` drops nil from the result type."},
			{Name: "not", Kind: EntryKeyword, Signature: "not a",
				Summary: "Logical negation; yields a boolean. Propagates type narrowing to the negated branch."},
			{Name: "nil", Kind: EntryConstant, Signature: "nil",
				Summary: "The absent value. Only nil and false are falsy."},
			{Name: "true", Kind: EntryConstant, Signature: "true",
				Summary: "The true boolean."},
			{Name: "false", Kind: EntryConstant, Signature: "false",
				Summary: "The false boolean — one of the two falsy values, along with nil."},
		},
	},
}
