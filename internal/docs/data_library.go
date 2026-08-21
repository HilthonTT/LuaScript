package docs

// Auto-global namespaces — installed into _G by internal/vm/stdlib_modules.go
// and internal/vm/coroutine.go, so they need no require.
//
// Note that `math` and `io` exist twice with different surfaces: the small
// auto-global documented here, and a larger native module of the same name
// reached through require. Where they differ the module page says so.

var libraryTopics = []Topic{
	{
		Name:          "string",
		Kind:          KindLibrary,
		RuntimeGlobal: "string",
		Title:         "string manipulation and Lua patterns",
		Synopsis:      `string.upper("hi")   -- auto-global, no require`,
		Detail: `Indices are 1-based and may be negative to count from the end. The
pattern functions implement the full Lua 5.4 pattern surface: the classes
%a %c %d %g %l %p %s %u %w %x and their upper-case complements, [sets]
with ranges, the anchors ^ and $, the quantifiers * + - ?, captures
including empty position captures, %1..%9 backreferences in both patterns
and replacements, %b() balanced matches and %f[set] frontiers.

Lua patterns are not regular expressions. For RE2 syntax, require the
regexp module instead.`,
		Example: `local s = "key = value"
local k, v = string.match(s, "(%w+)%s*=%s*(%w+)")
print(string.format("%s -> %s", k, v))

for word in string.gmatch("a,b,c", "[^,]+") do print(word) end`,
		SeeAlso: []string{"regexp", "utf8", "table"},
		Entries: []Entry{
			{Name: "len", Kind: EntryFunction, Signature: "string.len(s): number",
				Summary: "Length of s in bytes. The # operator does the same thing."},
			{Name: "sub", Kind: EntryFunction, Signature: "string.sub(s, i [, j]): string",
				Summary: "Substring of s from index i through j (default -1). Negative indices count from the end."},
			{Name: "upper", Kind: EntryFunction, Signature: "string.upper(s): string",
				Summary: "s with every letter upper-cased."},
			{Name: "lower", Kind: EntryFunction, Signature: "string.lower(s): string",
				Summary: "s with every letter lower-cased."},
			{Name: "reverse", Kind: EntryFunction, Signature: "string.reverse(s): string",
				Summary: "s with its bytes reversed."},
			{Name: "rep", Kind: EntryFunction, Signature: "string.rep(s, n [, sep]): string",
				Summary: "s repeated n times, joined by sep when given."},
			{Name: "byte", Kind: EntryFunction, Signature: "string.byte(s [, i [, j]]): ...number",
				Summary: "The numeric byte codes of s[i..j], returned as multiple values."},
			{Name: "char", Kind: EntryFunction, Signature: "string.char(...): string",
				Summary: "Builds a string from the given byte codes."},
			{Name: "format", Kind: EntryFunction, Signature: "string.format(fmt, ...): string",
				Summary: `printf-style formatting: %d, %i, %s, %q, %x, %X, %o, %c, %e, %f, %g and %% , with the usual width and precision modifiers.`},
			{Name: "find", Kind: EntryFunction, Signature: "string.find(s, pat [, init [, plain]]): number?, number?, ...",
				Summary: "Searches s for pat and returns the start and end indices of the first match, plus any captures. Returns nil when there is no match.",
				Detail:  "A pattern with no magic characters takes a plain-substring fast path; passing plain = true forces it."},
			{Name: "match", Kind: EntryFunction, Signature: "string.match(s, pat [, init]): ...",
				Summary: "Returns the captures of the first match of pat in s — or the whole match when the pattern has no captures. nil when there is no match."},
			{Name: "gmatch", Kind: EntryFunction, Signature: "string.gmatch(s, pat): iterator",
				Summary: "Returns an iterator yielding the captures of every match of pat in s. Intended for a generic for loop."},
			{Name: "gsub", Kind: EntryFunction, Signature: "string.gsub(s, pat, repl [, n]): string, number",
				Summary: "Replaces up to n matches of pat in s, returning the new string and the number of substitutions made.",
				Detail:  "repl may be a string (%1..%9 and %0 expand to captures), a table indexed by the first capture, or a function called with the captures."},
			{Name: "pack", Kind: EntryFunction, Signature: "string.pack(fmt, ...): string",
				Summary: "Packs the given values into a binary string according to the format string fmt.",
				Detail: `Format options: <, > and = select byte order (little, big, native);
! [n] sets the alignment ceiling; b/B a signed/unsigned byte; h/H a
2-byte short; l/L, j/J and T 8-byte integers; i[n]/I[n] an integer of n
bytes (default 4); f a 4-byte float; d and n an 8-byte double; s[n] a
string with an n-byte length prefix (default 8); z a zero-terminated
string; c[n] a fixed n-byte string; x one padding byte; X aligns to the
option that follows it; spaces are ignored.

Values that do not fit their slot raise rather than being truncated.

Note: l/L are fixed at 8 bytes on every platform, where PUC Lua sizes
them with C's sizeof(long) — 8 on Unix, 4 on Windows. Use i4/i8 when
the width has to be explicit.`},
			{Name: "unpack", Kind: EntryFunction, Signature: "string.unpack(fmt, s [, pos]): ...",
				Summary: "Reads values back out of a binary string packed with string.pack, returning them followed by the position of the first unread byte.",
				Detail:  "pos defaults to 1 and may be negative to count from the end. The trailing position makes it easy to walk a buffer with successive calls."},
			{Name: "packsize", Kind: EntryFunction, Signature: "string.packsize(fmt): number",
				Summary: "Returns the byte size of a packed string for fmt, including any alignment padding.",
				Detail:  "Raises for formats containing s or z, whose size depends on the values being packed."},
		},
	},
	{
		Name:          "table",
		Kind:          KindLibrary,
		RuntimeGlobal: "table",
		Title:         "array and table utilities",
		Synopsis:      `table.insert(t, v)   -- auto-global, no require`,
		Detail: `These operate on the array part of a table — the contiguous run of
integer keys starting at 1. The length of that run is what # reports.`,
		SeeAlso: []string{"sort", "std", "string"},
		Entries: []Entry{
			{Name: "insert", Kind: EntryFunction, Signature: "table.insert(t, [pos,] v)",
				Summary: "Appends v to t, or inserts it at pos and shifts the later elements up."},
			{Name: "remove", Kind: EntryFunction, Signature: "table.remove(t [, pos]): any",
				Summary: "Removes and returns the element at pos (default: the last), shifting the later elements down."},
			{Name: "concat", Kind: EntryFunction, Signature: "table.concat(t [, sep [, i [, j]]]): string",
				Summary: "Joins t[i..j] into a string separated by sep. Elements must be strings or numbers."},
			{Name: "unpack", Kind: EntryFunction, Signature: "table.unpack(t [, i [, j]]): ...",
				Summary: "Returns t[i..j] as multiple values."},
			{Name: "pack", Kind: EntryFunction, Signature: "table.pack(...): table",
				Summary: `Packs its arguments into a new table with a field "n" holding the count.`},
			{Name: "move", Kind: EntryFunction, Signature: "table.move(a1, f, e, t [, a2]): table",
				Summary: "Copies a1[f..e] to a2[t...] (a2 defaults to a1) and returns the destination. Handles overlapping ranges correctly."},
			{Name: "sort", Kind: EntryFunction, Signature: "table.sort(t [, cmp])",
				Summary: "Sorts t's array part in place. cmp(a, b) must return true when a comes strictly before b.",
				Detail:  "The sort module offers a stable variant plus several teaching implementations."},
		},
	},
	{
		Name:          "math",
		Kind:          KindLibrary,
		Requireable:   true,
		RuntimeModule: "math",
		RuntimeGlobal: "math",
		Title:         "numeric functions and constants",
		Synopsis: `math.floor(2.7)                  -- auto-global, always present
local math = require("math")     -- larger surface, same name`,
		Detail: `math exists twice with different surfaces, and this page documents the
union. The auto-global table is the Lua 5.4 surface. require("math")
returns a bigger table — hyperbolics, cbrt, erf, deg/rad, clamp, softmax,
statistical helpers and a full set of numeric limits — but it drops
randomseed, maxinteger and mininteger. Entries that live on only one of
the two say so.

Integers and floats are distinct subtypes, as in Lua 5.4: math.type
tells them apart and integer arithmetic stays exact.`,
		SeeAlso: []string{"stats", "bit32", "ndarray", "linalg"},
		Entries: []Entry{
			{Name: "floor", Kind: EntryFunction, Signature: "math.floor(x): number",
				Summary: "The largest integer not greater than x."},
			{Name: "ceil", Kind: EntryFunction, Signature: "math.ceil(x): number",
				Summary: "The smallest integer not less than x."},
			{Name: "abs", Kind: EntryFunction, Signature: "math.abs(x): number",
				Summary: "Absolute value of x, preserving the integer subtype."},
			{Name: "sqrt", Kind: EntryFunction, Signature: "math.sqrt(x): number",
				Summary: "Square root of x."},
			{Name: "exp", Kind: EntryFunction, Signature: "math.exp(x): number",
				Summary: "e raised to the power x."},
			{Name: "log", Kind: EntryFunction, Signature: "math.log(x [, base]): number",
				Summary: "Natural logarithm of x, or its logarithm in the given base."},
			{Name: "pow", Kind: EntryFunction, Signature: "math.pow(x, y): number",
				Summary: "x raised to the power y. The ^ operator is the idiomatic form."},
			{Name: "fmod", Kind: EntryFunction, Signature: "math.fmod(x, y): number",
				Summary: "Floating-point remainder of x / y, taking the sign of x. Differs from the % operator, which floors."},
			{Name: "modf", Kind: EntryFunction, Signature: "math.modf(x): number, number",
				Summary: "Splits x into its integral and fractional parts and returns both."},
			{Name: "sin", Kind: EntryFunction, Signature: "math.sin(x): number", Summary: "Sine of x, in radians."},
			{Name: "cos", Kind: EntryFunction, Signature: "math.cos(x): number", Summary: "Cosine of x, in radians."},
			{Name: "tan", Kind: EntryFunction, Signature: "math.tan(x): number", Summary: "Tangent of x, in radians."},
			{Name: "asin", Kind: EntryFunction, Signature: "math.asin(x): number", Summary: "Arc sine of x, in radians."},
			{Name: "acos", Kind: EntryFunction, Signature: "math.acos(x): number", Summary: "Arc cosine of x, in radians."},
			{Name: "atan", Kind: EntryFunction, Signature: "math.atan(y [, x]): number",
				Summary: "Arc tangent of y/x in radians, using the signs of both arguments to pick the quadrant."},
			{Name: "max", Kind: EntryFunction, Signature: "math.max(...): number", Summary: "The largest of its arguments."},
			{Name: "min", Kind: EntryFunction, Signature: "math.min(...): number", Summary: "The smallest of its arguments."},
			{Name: "tointeger", Kind: EntryFunction, Signature: "math.tointeger(v): number?",
				Summary: "Returns v as an integer when it has an exact integer value, otherwise nil."},
			{Name: "type", Kind: EntryFunction, Signature: `math.type(v): string?`,
				Summary: `Returns "integer" or "float" for a number, and nil for anything else.`,
				Detail:  `Auto-global only — the math module does not export it.`},
			{Name: "random", Kind: EntryFunction, Signature: "math.random([m [, n]]): number",
				Summary: "A float in [0,1) with no arguments, an integer in [1,m] with one, or an integer in [m,n] with two."},
			{Name: "randomseed", Kind: EntryFunction, Signature: "math.randomseed([x]): number, number",
				Summary: "Seeds the pseudo-random generator and returns the seed components.",
				Detail:  "Auto-global only — the math module does not export it."},
			{Name: "ult", Kind: EntryFunction, Signature: "math.ult(m, n): boolean",
				Summary: "Compares m and n as unsigned integers.", Detail: "Module only."},
			{Name: "sinh", Kind: EntryFunction, Signature: "math.sinh(x): number", Summary: "Hyperbolic sine of x.", Detail: "Module only."},
			{Name: "cosh", Kind: EntryFunction, Signature: "math.cosh(x): number", Summary: "Hyperbolic cosine of x.", Detail: "Module only."},
			{Name: "tanh", Kind: EntryFunction, Signature: "math.tanh(x): number", Summary: "Hyperbolic tangent of x.", Detail: "Module only."},
			{Name: "asinh", Kind: EntryFunction, Signature: "math.asinh(x): number", Summary: "Inverse hyperbolic sine of x.", Detail: "Module only."},
			{Name: "acosh", Kind: EntryFunction, Signature: "math.acosh(x): number", Summary: "Inverse hyperbolic cosine of x.", Detail: "Module only."},
			{Name: "atanh", Kind: EntryFunction, Signature: "math.atanh(x): number", Summary: "Inverse hyperbolic tangent of x.", Detail: "Module only."},
			{Name: "cbrt", Kind: EntryFunction, Signature: "math.cbrt(x): number", Summary: "Cube root of x.", Detail: "Module only."},
			{Name: "erf", Kind: EntryFunction, Signature: "math.erf(x): number", Summary: "The error function of x.", Detail: "Module only."},
			{Name: "erfc", Kind: EntryFunction, Signature: "math.erfc(x): number", Summary: "The complementary error function of x.", Detail: "Module only."},
			{Name: "deg", Kind: EntryFunction, Signature: "math.deg(x): number", Summary: "Converts x from radians to degrees.", Detail: "Module only."},
			{Name: "rad", Kind: EntryFunction, Signature: "math.rad(x): number", Summary: "Converts x from degrees to radians.", Detail: "Module only."},
			{Name: "clamp", Kind: EntryFunction, Signature: "math.clamp(x, min, max): number",
				Summary: "Constrains x to the range [min, max].", Detail: "Module only."},
			{Name: "mean", Kind: EntryFunction, Signature: "math.mean(t | ...): number",
				Summary: "Arithmetic mean of a table of numbers, or of the numbers passed directly.", Detail: "Module only. See also stats.mean."},
			{Name: "variance", Kind: EntryFunction, Signature: "math.variance(t): number",
				Summary: "Variance of the numbers in t.", Detail: "Module only."},
			{Name: "standard_deviation", Kind: EntryFunction, Signature: "math.standard_deviation(t): number",
				Summary: "Standard deviation of the numbers in t.", Detail: "Module only."},
			{Name: "softmax", Kind: EntryFunction, Signature: "math.softmax(t): table",
				Summary: "The softmax of the numbers in t — exponentials normalised to sum to 1.", Detail: "Module only."},

			{Name: "pi", Kind: EntryConstant, Signature: "math.pi: number", Summary: "The ratio of a circle's circumference to its diameter."},
			{Name: "huge", Kind: EntryConstant, Signature: "math.huge: number", Summary: "Positive infinity — greater than any other number."},
			{Name: "maxinteger", Kind: EntryConstant, Signature: "math.maxinteger: number",
				Summary: "The largest representable integer.", Detail: "Auto-global only; the module calls this maxint64."},
			{Name: "mininteger", Kind: EntryConstant, Signature: "math.mininteger: number",
				Summary: "The smallest representable integer.", Detail: "Auto-global only; the module calls this minint64."},
			{Name: "e", Kind: EntryConstant, Signature: "math.e: number", Summary: "Euler's number.", Detail: "Module only."},
			{Name: "phi", Kind: EntryConstant, Signature: "math.phi: number", Summary: "The golden ratio.", Detail: "Module only."},
			{Name: "nan", Kind: EntryConstant, Signature: "math.nan: number",
				Summary: "Not-a-number. It compares unequal to everything, including itself.", Detail: "Module only."},
			{Name: "sqrt2", Kind: EntryConstant, Signature: "math.sqrt2: number", Summary: "The square root of 2.", Detail: "Module only."},
			{Name: "sqrte", Kind: EntryConstant, Signature: "math.sqrte: number", Summary: "The square root of e.", Detail: "Module only."},
			{Name: "sqrtpi", Kind: EntryConstant, Signature: "math.sqrtpi: number", Summary: "The square root of π.", Detail: "Module only."},
			{Name: "sqrtphi", Kind: EntryConstant, Signature: "math.sqrtphi: number", Summary: "The square root of the golden ratio.", Detail: "Module only."},
			{Name: "ln2", Kind: EntryConstant, Signature: "math.ln2: number", Summary: "The natural logarithm of 2.", Detail: "Module only."},
			{Name: "ln10", Kind: EntryConstant, Signature: "math.ln10: number", Summary: "The natural logarithm of 10.", Detail: "Module only."},
			{Name: "log2e", Kind: EntryConstant, Signature: "math.log2e: number", Summary: "The base-2 logarithm of e.", Detail: "Module only."},
			{Name: "log10e", Kind: EntryConstant, Signature: "math.log10e: number", Summary: "The base-10 logarithm of e.", Detail: "Module only."},
			{Name: "maxfloat32", Kind: EntryConstant, Signature: "math.maxfloat32: number",
				Summary: "The largest finite 32-bit float.", Detail: "Module only."},
			{Name: "maxfloat64", Kind: EntryConstant, Signature: "math.maxfloat64: number",
				Summary: "The largest finite 64-bit float.", Detail: "Module only."},
			{Name: "smallestnonzerofloat32", Kind: EntryConstant, Signature: "math.smallestnonzerofloat32: number",
				Summary: "The smallest positive non-zero 32-bit float.", Detail: "Module only."},
			{Name: "smallestnonzerofloat64", Kind: EntryConstant, Signature: "math.smallestnonzerofloat64: number",
				Summary: "The smallest positive non-zero 64-bit float.", Detail: "Module only."},
			{Name: "maxint", Kind: EntryConstant, Signature: "math.maxint: number",
				Summary: "The largest platform int.",
				Detail:  "Module only. maxint8/16/32/64, minint, minint8/16/32/64 and maxuint8/16/32 are all present too."},
			{Name: "minint", Kind: EntryConstant, Signature: "math.minint: number",
				Summary: "The smallest platform int.", Detail: "Module only."},
			{Name: "maxint8", Kind: EntryConstant, Signature: "math.maxint8: number", Summary: "127.", Detail: "Module only."},
			{Name: "minint8", Kind: EntryConstant, Signature: "math.minint8: number", Summary: "-128.", Detail: "Module only."},
			{Name: "maxint16", Kind: EntryConstant, Signature: "math.maxint16: number", Summary: "32767.", Detail: "Module only."},
			{Name: "minint16", Kind: EntryConstant, Signature: "math.minint16: number", Summary: "-32768.", Detail: "Module only."},
			{Name: "maxint32", Kind: EntryConstant, Signature: "math.maxint32: number", Summary: "2147483647.", Detail: "Module only."},
			{Name: "minint32", Kind: EntryConstant, Signature: "math.minint32: number", Summary: "-2147483648.", Detail: "Module only."},
			{Name: "maxint64", Kind: EntryConstant, Signature: "math.maxint64: number",
				Summary: "The largest 64-bit signed integer — the module's name for maxinteger.", Detail: "Module only."},
			{Name: "minint64", Kind: EntryConstant, Signature: "math.minint64: number",
				Summary: "The smallest 64-bit signed integer — the module's name for mininteger.", Detail: "Module only."},
			{Name: "maxuint8", Kind: EntryConstant, Signature: "math.maxuint8: number", Summary: "255.", Detail: "Module only."},
			{Name: "maxuint16", Kind: EntryConstant, Signature: "math.maxuint16: number", Summary: "65535.", Detail: "Module only."},
			{Name: "maxuint32", Kind: EntryConstant, Signature: "math.maxuint32: number", Summary: "4294967295.", Detail: "Module only."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "math.VERSION: string",
				Summary: "The module's version string.", Detail: "Module only."},
		},
	},
	{
		Name:          "coroutine",
		Kind:          KindLibrary,
		RuntimeGlobal: "coroutine",
		Title:         "cooperative multitasking",
		Synopsis:      `local co = coroutine.create(f)   -- auto-global, no require`,
		Detail: `Coroutines are implemented on goroutines synchronised by channels, but
they behave like Lua's: exactly one runs at a time and control passes
only at an explicit resume or yield. A try/catch region travels with the
coroutine's frames across a yield.

For real concurrency — background timers, worker fan-out — use the queue
module, which keeps every Lua call on the VM goroutine.`,
		Example: `local co = coroutine.create(function(a)
  local b = coroutine.yield(a + 1)
  return b * 2
end)
print(coroutine.resume(co, 1))   --> true  2
print(coroutine.resume(co, 10))  --> true  20`,
		SeeAlso: []string{"queue", "_G"},
		Entries: []Entry{
			{Name: "create", Kind: EntryFunction, Signature: "coroutine.create(f): thread",
				Summary: "Creates a coroutine with body f. It does not start running until the first resume."},
			{Name: "resume", Kind: EntryFunction, Signature: "coroutine.resume(co, ...): boolean, ...",
				Summary: "Starts or continues co, passing the extra arguments in. Returns true plus the yielded or returned values, or false plus an error."},
			{Name: "yield", Kind: EntryFunction, Signature: "coroutine.yield(...): ...",
				Summary: "Suspends the running coroutine and hands its arguments to the resumer. Returns whatever the next resume passes in."},
			{Name: "status", Kind: EntryFunction, Signature: "coroutine.status(co): string",
				Summary: `One of "running", "suspended", "normal" or "dead".`},
			{Name: "wrap", Kind: EntryFunction, Signature: "coroutine.wrap(f): function",
				Summary: "Wraps a coroutine in a function that resumes it on each call. Errors propagate to the caller instead of being returned."},
			{Name: "running", Kind: EntryFunction, Signature: "coroutine.running(): thread?, boolean",
				Summary: "Returns the running coroutine plus a flag that is true when it is the main one."},
			{Name: "isyieldable", Kind: EntryFunction, Signature: "coroutine.isyieldable(): boolean",
				Summary: "True when the running coroutine can yield — that is, when it is not the main one."},
			{Name: "close", Kind: EntryFunction, Signature: "coroutine.close(co): boolean, any",
				Summary: "Closes a suspended or dead coroutine and releases its resources."},
		},
	},
	{
		Name:          "io",
		Kind:          KindLibrary,
		Requireable:   true,
		RuntimeModule: "io",
		RuntimeGlobal: "io",
		Title:         "file and stream I/O",
		Synopsis: `io.write("no newline")        -- auto-global, no require needed
local io = require("io")      -- the same library, named explicitly`,
		Detail: `Lua 5.4 exposes io without a require, and so does luascript: the global
io reaches the full library. require("io") returns the same module, and
is worth writing when you want the dependency to be visible.

The global table itself holds only the standard streams; everything else
is reached through its metatable, so iterating it with pairs lists far
less than io actually provides.

io.open returns a file handle whose methods are documented under
io.file. For path manipulation, directory creation and process-level
facilities, use the os module instead.`,
		Example: `local io = require("io")
local f = assert(io.open("data.txt", "w"))
f:write("line one\n")
f:close()

for line in io.lines("data.txt") do print(line) end`,
		SeeAlso: []string{"io.file", "os", "csv"},
		Entries: []Entry{
			{Name: "write", Kind: EntryFunction, Signature: "io.write(...)",
				Summary: "Writes each argument to stdout with no separator and no trailing newline. Honours __tostring."},
			{Name: "read", Kind: EntryFunction, Signature: `io.read([fmt]): string?`,
				Summary: `Reads from stdin: "l" (default) a line without its newline, "L" a line with it, "n" a number, "a" everything.`},
			{Name: "open", Kind: EntryFunction, Signature: `io.open(path [, mode]): file?, string?`,
				Summary: `Opens a file and returns a handle, or nil plus an error message. mode is "r" (default), "w", "a", "r+", "w+" or "a+", optionally with a "b".`,
				Detail:  "Module only."},
			{Name: "lines", Kind: EntryFunction, Signature: "io.lines([path]): iterator",
				Summary: "Returns an iterator over the lines of a file, or over stdin when no path is given. The file is closed when the iterator is exhausted.",
				Detail:  "Module only."},
			{Name: "close", Kind: EntryFunction, Signature: "io.close([file]): boolean",
				Summary: "Closes a file handle, or the current output file when called with no argument.", Detail: "Module only."},
			{Name: "flush", Kind: EntryFunction, Signature: "io.flush()",
				Summary: "Flushes the current output file.", Detail: "Module only."},
			{Name: "input", Kind: EntryFunction, Signature: "io.input([file | path]): file",
				Summary: "Gets or sets the default input file used by io.read.", Detail: "Module only."},
			{Name: "output", Kind: EntryFunction, Signature: "io.output([file | path]): file",
				Summary: "Gets or sets the default output file used by io.write.", Detail: "Module only."},
			{Name: "tmpfile", Kind: EntryFunction, Signature: "io.tmpfile(): file?, string?",
				Summary: "Opens a handle on a temporary file that is removed when the program ends.", Detail: "Module only."},
			{Name: "type", Kind: EntryFunction, Signature: `io.type(v): string?`,
				Summary: `Returns "file" for an open handle, "closed file" for a closed one, and nil for anything else.`,
				Detail:  "Module only."},
			{Name: "stdin", Kind: EntryField, Signature: "io.stdin: file",
				Summary: "The standard input stream, as a file handle.", Detail: "Module only."},
			{Name: "stdout", Kind: EntryField, Signature: "io.stdout: file",
				Summary: "The standard output stream, as a file handle.", Detail: "Module only."},
			{Name: "stderr", Kind: EntryField, Signature: "io.stderr: file",
				Summary: "The standard error stream, as a file handle.", Detail: "Module only."},
		},
	},
}
