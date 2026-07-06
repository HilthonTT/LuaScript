package server

import "github.com/hilthontt/luascript/internal/lsp/protocol"

// builtin describes one completable / hoverable identifier known to the
// language: a keyword, a global function, or a native module. The data is
// hand-maintained (there is no reflection over the VM's global table from
// here), so it is a curated snapshot rather than an exhaustive list.
type builtin struct {
	label  string
	kind   protocol.CompletionItemKind
	detail string
	doc    string
}

// keywords are the Lua 5.4 + luascript reserved words (compiler/token).
var keywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
	"match", "enum", "defer",
}

// globals are the builtin global functions installed by vm/stdlib.go.
var globals = []builtin{
	{"print", protocol.CompletionItemKindFunction, "print(...)", "Writes each argument to stdout separated by tabs, followed by a newline. Honours `__tostring`."},
	{"type", protocol.CompletionItemKindFunction, "type(v): string", "Returns the type name of `v` as a string (`\"nil\"`, `\"number\"`, `\"string\"`, `\"table\"`, ...)."},
	{"typeof", protocol.CompletionItemKindFunction, "typeof(v): string", "luascript extension: richer runtime type name than `type`."},
	{"sizeof", protocol.CompletionItemKindFunction, "sizeof(v): number", "luascript extension: size/length of a value."},
	{"tostring", protocol.CompletionItemKindFunction, "tostring(v): string", "Converts `v` to a string, routing through the `__tostring` metamethod when present."},
	{"tonumber", protocol.CompletionItemKindFunction, "tonumber(v [, base]): number?", "Converts `v` to a number, optionally in the given base. Returns nil on failure."},
	{"ipairs", protocol.CompletionItemKindFunction, "ipairs(t): iterator", "Returns an iterator over the array part of `t` (1..n)."},
	{"pairs", protocol.CompletionItemKindFunction, "pairs(t): iterator", "Returns an iterator over every key/value pair of `t`."},
	{"next", protocol.CompletionItemKindFunction, "next(t [, key])", "Returns the next key/value pair after `key`, used to traverse a table."},
	{"error", protocol.CompletionItemKindFunction, "error(message [, level])", "Raises an error with the given value. Unwinds until caught by `pcall`."},
	{"assert", protocol.CompletionItemKindFunction, "assert(v [, message])", "Raises an error when `v` is nil or false; otherwise returns all its arguments."},
	{"pcall", protocol.CompletionItemKindFunction, "pcall(f, ...): bool, ...", "Calls `f` in protected mode. Returns true plus results, or false plus the error."},
	{"select", protocol.CompletionItemKindFunction, "select(n, ...)", "Returns the arguments after index `n`, or the count when `n` is `\"#\"`."},
	{"collectgarbage", protocol.CompletionItemKindFunction, "collectgarbage([opt])", "Controls the garbage collector."},
	{"setmetatable", protocol.CompletionItemKindFunction, "setmetatable(t, mt): table", "Sets the metatable of `t` to `mt` and returns `t`."},
	{"getmetatable", protocol.CompletionItemKindFunction, "getmetatable(t): table?", "Returns the metatable of `t`, or nil."},
	{"rawget", protocol.CompletionItemKindFunction, "rawget(t, k)", "Reads `t[k]` without invoking the `__index` metamethod."},
	{"rawset", protocol.CompletionItemKindFunction, "rawset(t, k, v): table", "Writes `t[k] = v` without invoking the `__newindex` metamethod."},
	{"rawequal", protocol.CompletionItemKindFunction, "rawequal(a, b): bool", "Compares `a` and `b` without invoking the `__eq` metamethod."},
	{"rawlen", protocol.CompletionItemKindFunction, "rawlen(v): number", "Returns the length of a table or string without the `__len` metamethod."},
	{"require", protocol.CompletionItemKindFunction, "require(name): any", "Loads and returns the module `name`, resolving against `package.path` / native preloads."},
}

// modules are the native modules available via require(). Names come from
// cmd/natives.go::nativeRegistrars.
var modules = []builtin{
	{"math", protocol.CompletionItemKindModule, "require(\"math\")", "Numeric functions: `floor`, `ceil`, `abs`, `sqrt`, `min`, `max`, `clamp`, `random`, `pi`, ..."},
	{"string", protocol.CompletionItemKindModule, "string", "String library: `format`, `sub`, `find`, `match`, `gmatch`, `gsub`, `rep`, `byte`, `char`, ..."},
	{"table", protocol.CompletionItemKindModule, "table", "Table library: `insert`, `remove`, `concat`, `sort`, `unpack`, ..."},
	{"os", protocol.CompletionItemKindModule, "require(\"os\")", "Operating-system facilities: `time`, `clock`, `date`, `getenv`, `exit`, ..."},
	{"io", protocol.CompletionItemKindModule, "require(\"io\")", "Lua 5.4 io: `open`, `lines`, `read`, `write`, `stdin`, `stdout`, `stderr`, ..."},
	{"json", protocol.CompletionItemKindModule, "require(\"json\")", "JSON `encode` / `decode`."},
	{"http", protocol.CompletionItemKindModule, "require(\"http\")", "HTTP client."},
	{"httpserver", protocol.CompletionItemKindModule, "require(\"httpserver\")", "Blocking HTTP server with `:listen` / `:stop`."},
	{"crypto", protocol.CompletionItemKindModule, "require(\"crypto\")", "Hashing and cryptographic helpers."},
	{"time", protocol.CompletionItemKindModule, "require(\"time\")", "Time / duration utilities."},
	{"regexp", protocol.CompletionItemKindModule, "require(\"regexp\")", "RE2 regular expressions (`:capture`, not `:match`)."},
	{"uuid", protocol.CompletionItemKindModule, "require(\"uuid\")", "UUID generation."},
	{"sort", protocol.CompletionItemKindModule, "require(\"sort\")", "Sorting helpers."},
	{"compression", protocol.CompletionItemKindModule, "require(\"compression\")", "Compression codecs."},
	{"bit32", protocol.CompletionItemKindModule, "require(\"bit32\")", "32-bit bitwise operations."},
	{"utf8", protocol.CompletionItemKindModule, "require(\"utf8\")", "UTF-8 aware string helpers."},
	{"log", protocol.CompletionItemKindModule, "require(\"log\")", "Structured logging."},
	{"debug", protocol.CompletionItemKindModule, "require(\"debug\")", "`traceback` / `getinfo` plus hook stubs."},
	{"db", protocol.CompletionItemKindModule, "require(\"db\")", "Database access (Postgres via lib/pq)."},
	{"std", protocol.CompletionItemKindModule, "require(\"std\")", "Data structures: stacks, queues, deques, sets, lists, hashmaps, heaps, tries."},
	{"ndarray", protocol.CompletionItemKindModule, "require(\"ndarray\")", "Dense N-dimensional numeric arrays with broadcasting."},
	{"dataframe", protocol.CompletionItemKindModule, "require(\"dataframe\")", "Columnar pandas-lite data frame."},
	{"csv", protocol.CompletionItemKindModule, "require(\"csv\")", "CSV read / write."},
	{"stats", protocol.CompletionItemKindModule, "require(\"stats\")", "Descriptive statistics."},
	{"linalg", protocol.CompletionItemKindModule, "require(\"linalg\")", "Linear algebra."},
	{"clustering", protocol.CompletionItemKindModule, "require(\"clustering\")", "Clustering algorithms."},
	{"classification", protocol.CompletionItemKindModule, "require(\"classification\")", "Classification models."},
	{"ml", protocol.CompletionItemKindModule, "require(\"ml\")", "Machine-learning helpers."},
	{"plot", protocol.CompletionItemKindModule, "require(\"plot\")", "Dependency-free SVG charting."},
	{"ui", protocol.CompletionItemKindModule, "require(\"ui\")", "Fyne desktop GUI (real backend behind -tags luascript_ui)."},
}

// members maps a namespace (an auto-global library or a common native module)
// to the fields reachable through it via `ns.field`. The lists mirror the
// runtime surface — the auto-global math/string/table/io in
// vm/stdlib_modules.go (which also feeds compiler/typecheck/stdlib_types.go),
// coroutine from vm/coroutine.go, and the core of the os native. Members feed
// both dotted completion (`math.` -> floor, ceil, ...) and qualified hover
// (hovering `floor` in `math.floor`).
var members = map[string][]builtin{
	"math": {
		{"pi", protocol.CompletionItemKindConstant, "math.pi: number", "The value of π."},
		{"huge", protocol.CompletionItemKindConstant, "math.huge: number", "Positive infinity — larger than any other numeric value."},
		{"maxinteger", protocol.CompletionItemKindConstant, "math.maxinteger: number", "The largest representable integer."},
		{"mininteger", protocol.CompletionItemKindConstant, "math.mininteger: number", "The smallest representable integer."},
		{"abs", protocol.CompletionItemKindFunction, "math.abs(x): number", "Absolute value of `x`."},
		{"ceil", protocol.CompletionItemKindFunction, "math.ceil(x): number", "Smallest integer ≥ `x`."},
		{"floor", protocol.CompletionItemKindFunction, "math.floor(x): number", "Largest integer ≤ `x`."},
		{"sqrt", protocol.CompletionItemKindFunction, "math.sqrt(x): number", "Square root of `x`."},
		{"exp", protocol.CompletionItemKindFunction, "math.exp(x): number", "e raised to the power `x`."},
		{"log", protocol.CompletionItemKindFunction, "math.log(x [, base]): number", "Natural logarithm of `x`, or log base `base` when given."},
		{"sin", protocol.CompletionItemKindFunction, "math.sin(x): number", "Sine of `x` (radians)."},
		{"cos", protocol.CompletionItemKindFunction, "math.cos(x): number", "Cosine of `x` (radians)."},
		{"tan", protocol.CompletionItemKindFunction, "math.tan(x): number", "Tangent of `x` (radians)."},
		{"asin", protocol.CompletionItemKindFunction, "math.asin(x): number", "Arc sine of `x`, in radians."},
		{"acos", protocol.CompletionItemKindFunction, "math.acos(x): number", "Arc cosine of `x`, in radians."},
		{"atan", protocol.CompletionItemKindFunction, "math.atan(y [, x]): number", "Arc tangent of `y/x`, in radians, using the signs of both to find the quadrant."},
		{"fmod", protocol.CompletionItemKindFunction, "math.fmod(x, y): number", "Floating-point remainder of `x / y`, with the sign of `x`."},
		{"pow", protocol.CompletionItemKindFunction, "math.pow(x, y): number", "`x` raised to the power `y` (prefer the `^` operator)."},
		{"modf", protocol.CompletionItemKindFunction, "math.modf(x): number, number", "Splits `x` into its integral and fractional parts, returning both."},
		{"max", protocol.CompletionItemKindFunction, "math.max(...): number", "The largest of its numeric arguments."},
		{"min", protocol.CompletionItemKindFunction, "math.min(...): number", "The smallest of its numeric arguments."},
		{"tointeger", protocol.CompletionItemKindFunction, "math.tointeger(v): number?", "Returns `v` as an integer if it has an exact integer value, otherwise nil."},
		{"type", protocol.CompletionItemKindFunction, "math.type(v): string?", "Returns `\"integer\"` or `\"float\"` for a number, or nil for a non-number."},
		{"random", protocol.CompletionItemKindFunction, "math.random([m [, n]]): number", "Uniform pseudo-random: a float in [0,1) with no args, an integer in [1,m], or in [m,n]."},
		{"randomseed", protocol.CompletionItemKindFunction, "math.randomseed([x]): number, number", "Seeds the pseudo-random generator."},
	},
	"string": {
		{"len", protocol.CompletionItemKindFunction, "string.len(s): number", "Length of `s` in bytes (prefer the `#` operator)."},
		{"upper", protocol.CompletionItemKindFunction, "string.upper(s): string", "`s` with every letter upper-cased."},
		{"lower", protocol.CompletionItemKindFunction, "string.lower(s): string", "`s` with every letter lower-cased."},
		{"reverse", protocol.CompletionItemKindFunction, "string.reverse(s): string", "`s` reversed."},
		{"rep", protocol.CompletionItemKindFunction, "string.rep(s, n [, sep]): string", "`s` repeated `n` times, optionally joined by `sep`."},
		{"sub", protocol.CompletionItemKindFunction, "string.sub(s, i [, j]): string", "Substring of `s` from index `i` to `j` (negative indices count from the end)."},
		{"byte", protocol.CompletionItemKindFunction, "string.byte(s [, i [, j]]): ...number", "Numeric byte codes of the characters `s[i..j]`."},
		{"char", protocol.CompletionItemKindFunction, "string.char(...): string", "Builds a string from the given byte codes."},
		{"find", protocol.CompletionItemKindFunction, "string.find(s, pat [, init [, plain]]): number?, number?", "Searches `s` for pattern `pat`, returning the start and end indices of the first match, or nil."},
		{"format", protocol.CompletionItemKindFunction, "string.format(fmt, ...): string", "printf-style formatting: `%d`, `%s`, `%q`, `%x`, `%.2f`, ..."},
		{"match", protocol.CompletionItemKindFunction, "string.match(s, pat [, init]): ...any", "Returns the captures of the first match of `pat` in `s`, or nil."},
		{"gmatch", protocol.CompletionItemKindFunction, "string.gmatch(s, pat): iterator", "Iterator over every match of `pat` in `s`."},
		{"gsub", protocol.CompletionItemKindFunction, "string.gsub(s, pat, repl [, n]): string, number", "Replaces matches of `pat` in `s`, returning the result and the number of substitutions."},
	},
	"table": {
		{"insert", protocol.CompletionItemKindFunction, "table.insert(t, [pos,] v)", "Inserts `v` into `t`, at position `pos` (default: the end)."},
		{"remove", protocol.CompletionItemKindFunction, "table.remove(t [, pos]): any?", "Removes and returns the element at `pos` (default: the last)."},
		{"concat", protocol.CompletionItemKindFunction, "table.concat(t [, sep [, i [, j]]]): string", "Joins `t[i..j]` into a string separated by `sep`."},
		{"unpack", protocol.CompletionItemKindFunction, "table.unpack(t [, i [, j]]): ...any", "Returns the elements `t[i..j]` as multiple values."},
		{"pack", protocol.CompletionItemKindFunction, "table.pack(...): table", "Packs its arguments into a new table with an `n` field holding the count."},
	},
	"io": {
		{"write", protocol.CompletionItemKindFunction, "io.write(...)", "Writes each argument to stdout with no separator or trailing newline."},
		{"read", protocol.CompletionItemKindFunction, "io.read([fmt]): string?", "Reads from stdin according to `fmt` (`\"l\"` a line, `\"L\"` a line with its newline)."},
	},
	"coroutine": {
		{"create", protocol.CompletionItemKindFunction, "coroutine.create(f): thread", "Creates a new coroutine with body `f`."},
		{"resume", protocol.CompletionItemKindFunction, "coroutine.resume(co, ...): bool, ...", "Starts or resumes `co`, passing extra args to it. Returns success plus its yielded/returned values."},
		{"yield", protocol.CompletionItemKindFunction, "coroutine.yield(...): ...", "Suspends the running coroutine, returning its arguments to the resumer."},
		{"status", protocol.CompletionItemKindFunction, "coroutine.status(co): string", "One of `\"running\"`, `\"suspended\"`, `\"normal\"`, `\"dead\"`."},
		{"wrap", protocol.CompletionItemKindFunction, "coroutine.wrap(f): function", "Wraps a coroutine as a callable that resumes it on each call."},
		{"isyieldable", protocol.CompletionItemKindFunction, "coroutine.isyieldable(): bool", "True when the running coroutine can yield."},
	},
	"os": {
		{"time", protocol.CompletionItemKindFunction, "os.time([t]): number", "Current Unix time, or the time described by table `t`."},
		{"clock", protocol.CompletionItemKindFunction, "os.clock(): number", "CPU time used by the program, in seconds."},
		{"date", protocol.CompletionItemKindFunction, "os.date([fmt [, time]]): any", "Formats `time` (default: now) per the strftime-style `fmt`."},
		{"difftime", protocol.CompletionItemKindFunction, "os.difftime(t2, t1): number", "Seconds between two `os.time` values."},
		{"getenv", protocol.CompletionItemKindFunction, "os.getenv(name): string?", "Value of environment variable `name`, or nil."},
		{"setenv", protocol.CompletionItemKindFunction, "os.setenv(name, value)", "Sets environment variable `name`."},
		{"exit", protocol.CompletionItemKindFunction, "os.exit([code])", "Terminates the process with the given exit code (default 0)."},
		{"remove", protocol.CompletionItemKindFunction, "os.remove(path): bool, string?", "Deletes the file or empty directory at `path`."},
		{"rename", protocol.CompletionItemKindFunction, "os.rename(from, to): bool, string?", "Renames/moves a file."},
		{"mkdir", protocol.CompletionItemKindFunction, "os.mkdir(path): bool, string?", "Creates a directory."},
		{"tmpname", protocol.CompletionItemKindFunction, "os.tmpname(): string", "A name usable for a temporary file."},
		{"getcwd", protocol.CompletionItemKindFunction, "os.getcwd(): string", "The current working directory."},
		{"hostname", protocol.CompletionItemKindFunction, "os.hostname(): string", "The host machine's name."},
		{"execute", protocol.CompletionItemKindFunction, "os.execute([cmd]): any", "Runs `cmd` through the system shell."},
	},
}

// hoverDocs is the label -> markdown lookup used by textDocument/hover. It is
// assembled once from the same tables that feed completion so the two never
// drift apart. Keys are both bare names (`print`, `math`) and qualified member
// names (`math.floor`) so hover works on either half of a dotted expression.
var hoverDocs = buildHoverDocs()

func buildHoverDocs() map[string]string {
	m := make(map[string]string, len(globals)+len(modules)+len(keywords))
	render := func(b builtin) string {
		return "```luascript\n" + b.detail + "\n```\n\n" + b.doc
	}
	for _, b := range globals {
		m[b.label] = render(b)
	}
	for _, b := range modules {
		if _, exists := m[b.label]; exists {
			continue
		}
		m[b.label] = render(b)
	}
	for ns, ms := range members {
		for _, b := range ms {
			m[ns+"."+b.label] = render(b)
		}
	}
	for _, kw := range keywords {
		if _, exists := m[kw]; !exists {
			m[kw] = "`" + kw + "` — luascript keyword."
		}
	}
	return m
}

// memberCompletionItems returns the completion set for `ns.` — the fields of a
// known namespace — or nil when ns is not a namespace we model.
func memberCompletionItems(ns string) []protocol.CompletionItem {
	ms, ok := members[ns]
	if !ok {
		return nil
	}
	items := make([]protocol.CompletionItem, 0, len(ms))
	for _, b := range ms {
		items = append(items, protocol.CompletionItem{
			Label:  b.label,
			Kind:   b.kind,
			Detail: b.detail,
			Documentation: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: b.doc,
			},
		})
	}
	return items
}

// completionItems returns the static completion set: keywords, globals, and
// native module names.
func completionItems() []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, 0, len(keywords)+len(globals)+len(modules))
	for _, kw := range keywords {
		items = append(items, protocol.CompletionItem{
			Label:  kw,
			Kind:   protocol.CompletionItemKindKeyword,
			Detail: "keyword",
		})
	}
	seen := make(map[string]bool)
	add := func(bs []builtin) {
		for _, b := range bs {
			if seen[b.label] {
				continue
			}
			seen[b.label] = true
			items = append(items, protocol.CompletionItem{
				Label:  b.label,
				Kind:   b.kind,
				Detail: b.detail,
				Documentation: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: b.doc,
				},
			})
		}
	}
	add(globals)
	add(modules)
	return items
}
