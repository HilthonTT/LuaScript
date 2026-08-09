package docs

// Native modules reached through require(). Names and members mirror
// cmd/luascript/natives.go::nativeRegistrars and the tables each loader
// installs — cmd/luascript's TestDocsMatchRuntime checks that every Name
// below still exists at runtime.
//
// math and io are documented on their auto-global pages (data_library.go),
// which cover both surfaces.

var moduleTopics = []Topic{
	{
		Name: "os", Kind: KindModule, RuntimeModule: "os",
		Title:    "operating-system facilities",
		Synopsis: `local os = require("os")`,
		Detail: `os is NOT an auto-global in luascript — it must be required. Beyond the
Lua 5.4 surface it exposes low-level file opening (os.open with o_* flags
and mode_* permissions), directory and environment manipulation, and the
platform constants.

os.open and os.create return a file object documented under os.file. That
object is a different type from the one io.open returns: it is
descriptor-oriented (seek by whence constant, stat, chmod, truncate)
rather than stream-oriented.`,
		Example: `local os = require("os")
print(os.platform, os.arch)
print(os.date("%Y-%m-%d"))
local f = os.open("/tmp/x", os.o_create | os.o_wronly, 0644)
f:write("hi"); f:close()`,
		SeeAlso: []string{"os.file", "io", "time", "log"},
		Entries: []Entry{
			{Name: "time", Kind: EntryFunction, Signature: "os.time([t]): number",
				Summary: "The current Unix time, or the time described by a table with year, month, day and optionally hour, min and sec."},
			{Name: "date", Kind: EntryFunction, Signature: `os.date([fmt [, time]]): string | table`,
				Summary: `Formats a time with strftime-style directives. A fmt of "*t" returns a table with year, month, day, hour, min, sec, wday, yday and isdst instead.`},
			{Name: "clock", Kind: EntryFunction, Signature: "os.clock(): number",
				Summary: "CPU time used by the program so far, in seconds."},
			{Name: "difftime", Kind: EntryFunction, Signature: "os.difftime(t2, t1): number",
				Summary: "The number of seconds from t1 to t2."},
			{Name: "getenv", Kind: EntryFunction, Signature: "os.getenv(name): string?",
				Summary: "The value of an environment variable, or nil when it is unset."},
			{Name: "setenv", Kind: EntryFunction, Signature: "os.setenv(name, value): boolean, string?",
				Summary: "Sets an environment variable for this process and its children."},
			{Name: "getcwd", Kind: EntryFunction, Signature: "os.getcwd(): string",
				Summary: "The current working directory."},
			{Name: "pwd", Kind: EntryFunction, Signature: "os.pwd(): string",
				Summary: "The current working directory — an alias of getcwd."},
			{Name: "hostname", Kind: EntryFunction, Signature: "os.hostname(): string",
				Summary: "The host machine's name."},
			{Name: "exit", Kind: EntryFunction, Signature: "os.exit([code])",
				Summary: "Terminates the process immediately with the given exit code. Deferred blocks do not run."},
			{Name: "execute", Kind: EntryFunction, Signature: `os.execute([cmd]): boolean, string, number`,
				Summary: `Runs cmd through the system shell and reports success, "exit" and the exit status. With no argument it reports whether a shell is available.`},
			{Name: "open", Kind: EntryFunction, Signature: "os.open(path, flags, perm): file",
				Summary: "Opens a file descriptor with the given o_* flag set and permission bits, returning an os.file.",
				Detail:  "Combine flags with the bitwise or: os.o_create | os.o_wronly | os.o_trunc."},
			{Name: "create", Kind: EntryFunction, Signature: "os.create(path): file",
				Summary: "Creates or truncates a file and returns an os.file open for writing."},
			{Name: "remove", Kind: EntryFunction, Signature: "os.remove(path): boolean, string?",
				Summary: "Deletes a file or an empty directory."},
			{Name: "rename", Kind: EntryFunction, Signature: "os.rename(from, to): boolean, string?",
				Summary: "Renames or moves a file."},
			{Name: "mkdir", Kind: EntryFunction, Signature: "os.mkdir(path, perm): boolean, string?",
				Summary: "Creates a directory with the given permission bits."},
			{Name: "tmpname", Kind: EntryFunction, Signature: "os.tmpname(): string",
				Summary: "A path usable for a temporary file. The file is not created."},
			{Name: "setlocale", Kind: EntryFunction, Signature: `os.setlocale([locale]): string?`,
				Summary: "Present for Lua compatibility; Go has no locale switch, so it reports the current setting only."},
			{Name: "platform", Kind: EntryConstant, Signature: "os.platform: string",
				Summary: `The host OS: "windows", "linux", "darwin", ...`},
			{Name: "arch", Kind: EntryConstant, Signature: "os.arch: string",
				Summary: `The host architecture: "amd64", "arm64", ...`},
			{Name: "path_separator", Kind: EntryConstant, Signature: "os.path_separator: string",
				Summary: `The path element separator — "\\" on Windows, "/" elsewhere.`},
			{Name: "path_list_separator", Kind: EntryConstant, Signature: "os.path_list_separator: string",
				Summary: `The separator between paths in a list variable — ";" on Windows, ":" elsewhere.`},
			{Name: "dev_null", Kind: EntryConstant, Signature: "os.dev_null: string",
				Summary: "The path of the null device."},
			{Name: "o_rdonly", Kind: EntryConstant, Signature: "os.o_rdonly: number",
				Summary: "Open-for-reading flag for os.open.",
				Detail:  "The full set is o_rdonly, o_wronly, o_rdwr, o_append, o_create, o_excl, o_sync and o_trunc; combine them with |."},
			{Name: "o_wronly", Kind: EntryConstant, Signature: "os.o_wronly: number", Summary: "Open-for-writing flag."},
			{Name: "o_rdwr", Kind: EntryConstant, Signature: "os.o_rdwr: number", Summary: "Open-for-reading-and-writing flag."},
			{Name: "o_append", Kind: EntryConstant, Signature: "os.o_append: number", Summary: "Append-on-write flag."},
			{Name: "o_create", Kind: EntryConstant, Signature: "os.o_create: number", Summary: "Create-if-absent flag."},
			{Name: "o_excl", Kind: EntryConstant, Signature: "os.o_excl: number", Summary: "With o_create, fail when the file already exists."},
			{Name: "o_sync", Kind: EntryConstant, Signature: "os.o_sync: number", Summary: "Synchronous-I/O flag."},
			{Name: "o_trunc", Kind: EntryConstant, Signature: "os.o_trunc: number", Summary: "Truncate-on-open flag."},
			{Name: "mode_dir", Kind: EntryConstant, Signature: "os.mode_dir: number",
				Summary: "Directory bit of a file mode.",
				Detail: `The mode_* set mirrors Go's fs.FileMode bits: mode_dir, mode_append,
mode_exclusive, mode_temporary, mode_symlink, mode_device,
mode_named_pipe, mode_socket, mode_setuid, mode_setgid,
mode_char_device, mode_sticky, mode_type and mode_perm.`},
			{Name: "mode_perm", Kind: EntryConstant, Signature: "os.mode_perm: number",
				Summary: "Mask selecting the Unix permission bits of a file mode."},
			{Name: "mode_type", Kind: EntryConstant, Signature: "os.mode_type: number",
				Summary: "Mask selecting the file-type bits of a file mode."},
			{Name: "mode_symlink", Kind: EntryConstant, Signature: "os.mode_symlink: number", Summary: "Symbolic-link bit."},
			{Name: "mode_device", Kind: EntryConstant, Signature: "os.mode_device: number", Summary: "Device-file bit."},
			{Name: "mode_char_device", Kind: EntryConstant, Signature: "os.mode_char_device: number", Summary: "Character-device bit."},
			{Name: "mode_named_pipe", Kind: EntryConstant, Signature: "os.mode_named_pipe: number", Summary: "Named-pipe (FIFO) bit."},
			{Name: "mode_socket", Kind: EntryConstant, Signature: "os.mode_socket: number", Summary: "Socket bit."},
			{Name: "mode_append", Kind: EntryConstant, Signature: "os.mode_append: number", Summary: "Append-only bit."},
			{Name: "mode_exclusive", Kind: EntryConstant, Signature: "os.mode_exclusive: number", Summary: "Exclusive-use bit."},
			{Name: "mode_temporary", Kind: EntryConstant, Signature: "os.mode_temporary: number", Summary: "Temporary-file bit."},
			{Name: "mode_setuid", Kind: EntryConstant, Signature: "os.mode_setuid: number", Summary: "Set-user-ID bit."},
			{Name: "mode_setgid", Kind: EntryConstant, Signature: "os.mode_setgid: number", Summary: "Set-group-ID bit."},
			{Name: "mode_sticky", Kind: EntryConstant, Signature: "os.mode_sticky: number", Summary: "Sticky bit."},
			{Name: "seek_set", Kind: EntryConstant, Signature: "os.seek_set: number",
				Summary: "Whence value for file:seek — relative to the start of the file. See also seek_cur and seek_end."},
			{Name: "seek_cur", Kind: EntryConstant, Signature: "os.seek_cur: number",
				Summary: "Whence value for file:seek — relative to the current offset."},
			{Name: "seek_end", Kind: EntryConstant, Signature: "os.seek_end: number",
				Summary: "Whence value for file:seek — relative to the end of the file."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "os.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "json", Kind: KindModule, RuntimeModule: "json",
		Title:    "JSON encoding and decoding",
		Synopsis: `local json = require("json")`,
		Detail: `Decoding preserves integers where the source text allows it, so a
round trip through decode does not silently turn 1 into 1.0.

Lua has one table type for both objects and arrays, so encode picks by
shape: a table whose keys are exactly 1..n becomes a JSON array,
anything else becomes an object. An empty table therefore encodes as [].`,
		Example: `local json = require("json")
local s = json.encode({ name = "ada", tags = { "x", "y" } })
local t = json.decode(s)
print(t.name, #t.tags)`,
		SeeAlso: []string{"http", "csv", "dataframe"},
		Entries: []Entry{
			{Name: "encode", Kind: EntryFunction, Signature: "json.encode(value): string",
				Summary: "Serialises a Lua value to JSON text. Raises on values JSON cannot represent, such as functions."},
			{Name: "decode", Kind: EntryFunction, Signature: "json.decode(text): any",
				Summary: "Parses JSON text into Lua values. Raises on malformed input."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "json.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "http", Kind: KindModule, RuntimeModule: "http",
		Title:    "HTTP client",
		Synopsis: `local http = require("http")`,
		Detail: `Every call returns a response table with these fields:

  status       number   the HTTP status code
  status_text  string   the status line, e.g. "200 OK"
  body         string   the response body
  ok           boolean  true when status is 2xx
  headers      table    header name -> value

The shorthands take a URL directly; http.request takes an options table
with method, url, body, headers, query and timeout (in seconds). For
several requests against one host, new_client shares a base URL, default
headers and a connection pool.`,
		Example: `local http = require("http")
local res = http.get("https://example.com")
if res.ok then print(#res.body) end

local api = http.new_client({ base_url = "https://api.example.com",
                              headers = { Authorization = "Bearer t" } })
local r = api:post("/items", '{"a":1}')`,
		SeeAlso: []string{"http.client", "httpserver", "json"},
		Entries: []Entry{
			{Name: "get", Kind: EntryFunction, Signature: "http.get(url [, opts]): table",
				Summary: "Performs a GET request and returns the response table."},
			{Name: "head", Kind: EntryFunction, Signature: "http.head(url [, opts]): table",
				Summary: "Performs a HEAD request."},
			{Name: "options", Kind: EntryFunction, Signature: "http.options(url [, opts]): table",
				Summary: "Performs an OPTIONS request."},
			{Name: "delete", Kind: EntryFunction, Signature: "http.delete(url [, opts]): table",
				Summary: "Performs a DELETE request."},
			{Name: "post", Kind: EntryFunction, Signature: "http.post(url [, body [, opts]]): table",
				Summary: "Performs a POST request with the given body string."},
			{Name: "put", Kind: EntryFunction, Signature: "http.put(url [, body [, opts]]): table",
				Summary: "Performs a PUT request with the given body string."},
			{Name: "patch", Kind: EntryFunction, Signature: "http.patch(url [, body [, opts]]): table",
				Summary: "Performs a PATCH request with the given body string."},
			{Name: "request", Kind: EntryFunction, Signature: "http.request(opts): table",
				Summary: "The full-surface entry point. opts.url is required; method defaults to GET.",
				Detail:  "Recognised keys: method, url, body, headers, query and timeout (seconds)."},
			{Name: "new_client", Kind: EntryFunction, Signature: "http.new_client([opts]): client",
				Summary: "Creates a reusable client. opts may set base_url, headers and timeout.",
				Detail:  "Headers given here are sent on every request unless a per-call header overrides them."},
			{Name: "encode_url", Kind: EntryFunction, Signature: "http.encode_url(t): string",
				Summary: `Percent-encodes a table into a query string. An array-valued key repeats: {a = {1, 2}} becomes "a=1&a=2".`},
			{Name: "MethodGet", Kind: EntryConstant, Signature: "http.MethodGet: string",
				Summary: `The string "GET".`,
				Detail:  "MethodPost, MethodPut, MethodPatch, MethodDelete, MethodHead, MethodOptions and MethodTrace are also present."},
			{Name: "MethodPost", Kind: EntryConstant, Signature: "http.MethodPost: string", Summary: `The string "POST".`},
			{Name: "MethodPut", Kind: EntryConstant, Signature: "http.MethodPut: string", Summary: `The string "PUT".`},
			{Name: "MethodPatch", Kind: EntryConstant, Signature: "http.MethodPatch: string", Summary: `The string "PATCH".`},
			{Name: "MethodDelete", Kind: EntryConstant, Signature: "http.MethodDelete: string", Summary: `The string "DELETE".`},
			{Name: "MethodHead", Kind: EntryConstant, Signature: "http.MethodHead: string", Summary: `The string "HEAD".`},
			{Name: "MethodOptions", Kind: EntryConstant, Signature: "http.MethodOptions: string", Summary: `The string "OPTIONS".`},
			{Name: "MethodTrace", Kind: EntryConstant, Signature: "http.MethodTrace: string", Summary: `The string "TRACE".`},
			{Name: "VERSION", Kind: EntryConstant, Signature: "http.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "httpserver", Kind: KindModule, RuntimeModule: "httpserver",
		Title:    "blocking HTTP server",
		Synopsis: `local httpserver = require("httpserver")`,
		Detail: `The server is serialised against the VM: :listen blocks on the VM
goroutine and dispatches handlers one at a time through a buffered
channel. That is deliberate — the VM has no locks, so running Lua on two
goroutines would corrupt it. Handlers must therefore be quick; long work
belongs in a queue.

Request bodies are capped at 8 MiB; larger ones get a 413. :stop() shuts
the server down and :listen returns cleanly.`,
		Example: `local server = require("httpserver").new()
server:get("/hello", function(req)
  return { status = 200, body = "hi " .. req.query }
end)
server:listen(":8080")`,
		SeeAlso: []string{"httpserver.server", "http", "json", "queue"},
		Entries: []Entry{
			{Name: "new", Kind: EntryFunction, Signature: "httpserver.new(): server",
				Summary: "Creates a server with no routes. See the httpserver.server page for its methods."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "httpserver.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "crypto", Kind: KindModule, RuntimeModule: "crypto",
		Title:    "hashes, HMAC and encodings",
		Synopsis: `local crypto = require("crypto")`,
		Detail: `Every digest function takes a string and returns its lowercase hex
digest. md5 and sha1 are provided for interoperability with existing
formats — do not use them where collision resistance matters.

Compare secrets with constant_time_equal or hmac_verify rather than ==,
so the comparison does not leak length information through timing.`,
		Example: `local crypto = require("crypto")
print(crypto.sha256("hello"))
local mac = crypto.hmac_sha256("key", "message")
assert(crypto.hmac_verify("key", "message", mac))`,
		SeeAlso: []string{"uuid", "compression"},
		Entries: []Entry{
			{Name: "sha256", Kind: EntryFunction, Signature: "crypto.sha256(s): string",
				Summary: "The SHA-256 digest of s, hex-encoded."},
			{Name: "sha512", Kind: EntryFunction, Signature: "crypto.sha512(s): string",
				Summary: "The SHA-512 digest of s, hex-encoded."},
			{Name: "sha3", Kind: EntryFunction, Signature: "crypto.sha3(s): string",
				Summary: "The SHA3-256 digest of s, hex-encoded."},
			{Name: "sha1", Kind: EntryFunction, Signature: "crypto.sha1(s): string",
				Summary: "The SHA-1 digest of s, hex-encoded. Legacy interoperability only."},
			{Name: "md5", Kind: EntryFunction, Signature: "crypto.md5(s): string",
				Summary: "The MD5 digest of s, hex-encoded. Legacy interoperability only."},
			{Name: "hmac_sha256", Kind: EntryFunction, Signature: "crypto.hmac_sha256(key, msg): string",
				Summary: "The HMAC-SHA256 of msg under key, hex-encoded."},
			{Name: "hmac_verify", Kind: EntryFunction, Signature: "crypto.hmac_verify(key, msg, expected_hex): boolean",
				Summary: "Recomputes the HMAC of msg and compares it against expected_hex in constant time."},
			{Name: "constant_time_equal", Kind: EntryFunction, Signature: "crypto.constant_time_equal(a, b): boolean",
				Summary: "Compares two strings in constant time, so the comparison does not leak where they differ."},
			{Name: "random_bytes", Kind: EntryFunction, Signature: "crypto.random_bytes(n): string",
				Summary: "n cryptographically secure random bytes, as a string. Combine with hex_encode for a printable token."},
			{Name: "hex_encode", Kind: EntryFunction, Signature: "crypto.hex_encode(s): string", Summary: "Hex-encodes a string."},
			{Name: "hex_decode", Kind: EntryFunction, Signature: "crypto.hex_decode(s): string", Summary: "Decodes a hex string. Raises on invalid input."},
			{Name: "base64_encode", Kind: EntryFunction, Signature: "crypto.base64_encode(s): string", Summary: "Standard base64 encoding."},
			{Name: "base64_decode", Kind: EntryFunction, Signature: "crypto.base64_decode(s): string", Summary: "Decodes standard base64. Raises on invalid input."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "crypto.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "time", Kind: KindModule, RuntimeModule: "time",
		Title:    "clocks, formatting and sleeping",
		Synopsis: `local time = require("time")`,
		Detail: `Layouts are Go reference-time layouts, not strftime: the reference
instant is "2006-01-02 15:04:05". The module exports the common ones as
constants. os.date, by contrast, uses strftime-style directives.

time.sleep blocks the VM goroutine. To wait without stalling everything,
use queue.after or queue.tick.`,
		Example: `local time = require("time")
local now = time.now()
print(time.format(now, time.DATETIME))
local t = time.parse(time.DATE, "2026-01-31")`,
		SeeAlso: []string{"os", "queue"},
		Entries: []Entry{
			{Name: "now", Kind: EntryFunction, Signature: "time.now(): number", Summary: "The current Unix time in seconds."},
			{Name: "now_ms", Kind: EntryFunction, Signature: "time.now_ms(): number", Summary: "The current Unix time in milliseconds."},
			{Name: "clock", Kind: EntryFunction, Signature: "time.clock(): number",
				Summary: "A monotonic timestamp in seconds, suitable for measuring elapsed time."},
			{Name: "sleep", Kind: EntryFunction, Signature: "time.sleep(seconds)",
				Summary: "Blocks the VM for the given number of seconds. Fractions are allowed."},
			{Name: "date", Kind: EntryFunction, Signature: "time.date([unix]): table",
				Summary: "Breaks a Unix timestamp (default: now) into a table with year, month, day, hour, min, sec, wday and yday."},
			{Name: "format", Kind: EntryFunction, Signature: "time.format(unix [, layout]): string",
				Summary: "Formats a Unix timestamp with a Go layout, defaulting to RFC3339."},
			{Name: "parse", Kind: EntryFunction, Signature: "time.parse(layout, s): number",
				Summary: "Parses s according to layout and returns the Unix time. Raises when the text does not match."},
			{Name: "RFC3339", Kind: EntryConstant, Signature: "time.RFC3339: string",
				Summary: `The layout "2006-01-02T15:04:05Z07:00".`},
			{Name: "DATE", Kind: EntryConstant, Signature: "time.DATE: string", Summary: `The layout "2006-01-02".`},
			{Name: "DATETIME", Kind: EntryConstant, Signature: "time.DATETIME: string", Summary: `The layout "2006-01-02 15:04:05".`},
			{Name: "KITCHEN", Kind: EntryConstant, Signature: "time.KITCHEN: string", Summary: `The layout "3:04PM".`},
			{Name: "VERSION", Kind: EntryConstant, Signature: "time.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "regexp", Kind: KindModule, RuntimeModule: "regexp",
		Title:    "RE2 regular expressions",
		Synopsis: `local regexp = require("regexp")`,
		Detail: `These are RE2 patterns — the Go regexp syntax — not Lua patterns.
RE2 guarantees linear-time matching and therefore has no backreferences
or lookaround. For Lua patterns use string.match and friends.

The method is called :capture, not :match, because match is a keyword.`,
		Example: `local regexp = require("regexp")
local re = regexp.compile("(\\w+)@(\\w+)\\.com")
if re:test("ada@example.com") then
  local user, host = re:capture("ada@example.com")
end`,
		SeeAlso: []string{"regexp.regex", "string"},
		Entries: []Entry{
			{Name: "compile", Kind: EntryFunction, Signature: "regexp.compile(pattern): regex",
				Summary: "Compiles an RE2 pattern into a reusable regex object. Raises when the pattern is invalid."},
			{Name: "quote", Kind: EntryFunction, Signature: "regexp.quote(s): string",
				Summary: "Escapes every metacharacter in s so it matches literally."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "regexp.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "uuid", Kind: KindModule, RuntimeModule: "uuid",
		Title:    "UUID generation",
		Synopsis: `local uuid = require("uuid")`,
		SeeAlso:  []string{"crypto"},
		Entries: []Entry{
			{Name: "v4", Kind: EntryFunction, Signature: "uuid.v4(): string",
				Summary: "A random (version 4) UUID in the canonical 8-4-4-4-12 hex form."},
			{Name: "is_valid", Kind: EntryFunction, Signature: "uuid.is_valid(s): boolean",
				Summary: "Reports whether s is a well-formed UUID."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "uuid.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "sort", Kind: KindModule, RuntimeModule: "sort",
		Title:    "sorting algorithms and helpers",
		Synopsis: `local sort = require("sort")`,
		Detail: `sort.sort is the one to reach for; it is what table.sort uses. The
named algorithms (bubble, circle, simple, quicksort) are included for
teaching and benchmarking, not because they are faster.

Every function sorts the array part in place and returns the table.
Comparators follow the Lua convention: cmp(a, b) is true when a comes
strictly before b.`,
		Example: `local sort = require("sort")
local t = { 3, 1, 2 }
sort.sort(t, function(a, b) return a > b end)
print(sort.is_sorted(t, function(a, b) return a > b end))`,
		SeeAlso: []string{"table", "std"},
		Entries: []Entry{
			{Name: "sort", Kind: EntryFunction, Signature: "sort.sort(t [, cmp]): table",
				Summary: "Sorts t in place with an efficient unstable sort, ascending by default."},
			{Name: "stable", Kind: EntryFunction, Signature: "sort.stable(t [, cmp]): table",
				Summary: "Sorts t in place, preserving the relative order of elements the comparator calls equal."},
			{Name: "is_sorted", Kind: EntryFunction, Signature: "sort.is_sorted(t [, cmp]): boolean",
				Summary: "Reports whether t is already in order under cmp."},
			{Name: "reverse", Kind: EntryFunction, Signature: "sort.reverse(t): table",
				Summary: "Reverses the array part of t in place."},
			{Name: "quicksort", Kind: EntryFunction, Signature: "sort.quicksort(t): table",
				Summary: "Quicksort over numbers. A reference implementation; sort.sort is the practical choice."},
			{Name: "bubble", Kind: EntryFunction, Signature: "sort.bubble(t): table",
				Summary: "Bubble sort over numbers. Quadratic — for teaching only."},
			{Name: "circle", Kind: EntryFunction, Signature: "sort.circle(t): table",
				Summary: "Circle sort over numbers. A reference implementation."},
			{Name: "simple", Kind: EntryFunction, Signature: "sort.simple(t): table",
				Summary: "A straightforward comparison sort over numbers. A reference implementation."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "sort.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "std", Kind: KindModule, RuntimeModule: "std",
		Title:    "data structures",
		Synopsis: `local std = require("std")`,
		Detail: `Each constructor returns an object with methods; the objects have
pages of their own (std.stack, std.set, std.btree, ...). They store Lua
values directly, so anything the VM tracks can go in.

Where a plain table would do, a plain table is usually faster. These
earn their keep when you want the operations to be O(1) and explicit —
a deque's push_front, a heap's ordering, a trie's prefix search.`,
		Example: `local std = require("std")
local s = std.new_stack()
s:push(1); s:push(2)
print(s:pop(), s:size())

local h = std.new_heap(function(a, b) return a < b end)
h:push(5); h:push(1)
print(h:top())`,
		SeeAlso: []string{"std.stack", "std.queue", "std.deque", "std.set", "std.list",
			"std.hashmap", "std.heap", "std.trie", "std.btree", "table", "sort"},
		Entries: []Entry{
			{Name: "new_stack", Kind: EntryFunction, Signature: "std.new_stack(): stack",
				Summary: "A LIFO stack: push, pop, peek, size, empty, clear."},
			{Name: "new_queue", Kind: EntryFunction, Signature: "std.new_queue(): queue",
				Summary: "A FIFO queue: push, pop, peek, size, empty, clear.",
				Detail:  "Unrelated to the queue module, which schedules jobs."},
			{Name: "new_deque", Kind: EntryFunction, Signature: "std.new_deque(): deque",
				Summary: "A double-ended queue: push_front, push_back, pop_front, pop_back, front, back."},
			{Name: "new_set", Kind: EntryFunction, Signature: "std.new_set(): set",
				Summary: "A set of unique values: add, remove, contains, values, size, empty, clear."},
			{Name: "new_list", Kind: EntryFunction, Signature: "std.new_list(): list",
				Summary: "A doubly linked list: push/pop at both ends, front, back, to_array."},
			{Name: "new_hashmap", Kind: EntryFunction, Signature: "std.new_hashmap(): hashmap",
				Summary: "A key/value map with put, get, contains and size — usable with keys a table would coerce."},
			{Name: "new_heap", Kind: EntryFunction, Signature: "std.new_heap(cmp): heap",
				Summary: "A binary heap ordered by cmp(a, b), which must be true when a has priority over b.",
				Detail:  "Pass `function(a, b) return a < b end` for a min-heap."},
			{Name: "new_trie", Kind: EntryFunction, Signature: "std.new_trie(): trie",
				Summary: "A string prefix tree: insert, find, remove, compact, size, capacity."},
			{Name: "new_btree", Kind: EntryFunction, Signature: "std.new_btree([max_keys]): btree",
				Summary: "An ordered B-tree over number or string keys (max_keys defaults to 3).",
				Detail:  "The key type is fixed by the first insert; mixing types afterwards raises."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "std.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "queue", Kind: KindModule, RuntimeModule: "queue",
		Title:    "priority job queue and channels",
		Synopsis: `local queue = require("queue")`,
		Detail: `Jobs run ONLY on the VM goroutine, one at a time. Scheduling is
thread-safe and can happen from anywhere, but execution never is — the
VM has no locks, so two goroutines running Lua would corrupt it. This is
why there is no worker pool.

Consequences worth knowing: timeout_ms is a deadline on *starting* a job
(an expired job is dropped unrun; a running call cannot be preempted),
and :run() blocks the caller until the queue drains or :stop() is called.
Use :poll() to run only what is already due and return immediately.

queue.channel is a Go channel carrying Lua values, for handing data back
from the timer goroutines that after and tick create.`,
		Example: `local queue = require("queue")
local q = queue.new({ on_error = function(err) print("failed:", err) end })
q:push(function() print("high") end, { priority = 10 })
q:push(function() print("later") end, { delay_ms = 50 })
q:run()`,
		SeeAlso: []string{"queue.jobqueue", "queue.channel", "coroutine", "httpserver"},
		Entries: []Entry{
			{Name: "new", Kind: EntryFunction, Signature: "queue.new([opts]): jobqueue",
				Summary: "Creates a job queue. opts.on_error is called with the error value when a job raises.",
				Detail:  "Jobs are ordered by priority (higher first), then FIFO within a priority."},
			{Name: "channel", Kind: EntryFunction, Signature: "queue.channel([capacity]): channel",
				Summary: "Creates a channel carrying Lua values. Capacity 0 (the default) makes it unbuffered."},
			{Name: "after", Kind: EntryFunction, Signature: "queue.after(ms [, value]): channel",
				Summary: "Returns a channel that receives once, after ms milliseconds."},
			{Name: "tick", Kind: EntryFunction, Signature: "queue.tick(ms): channel",
				Summary: "Returns a channel that receives every ms milliseconds until it is closed."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "queue.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "compression", Kind: KindModule, RuntimeModule: "compression",
		Title:    "gzip, zlib, deflate, RLE and Huffman coding",
		Synopsis: `local compression = require("compression")`,
		Detail: `The gzip/zlib/deflate pairs are the practical codecs: they take a
string and an optional level from 1 (fastest) to 9 (smallest), and
return a string. RLE and the Huffman functions (encode, decode, codes,
symbol_count) are teaching implementations with their own formats — do
not expect them to interoperate with anything else.`,
		Example: `local z = require("compression")
local packed = z.gzip_encode(("hello "):rep(100), 9)
print(#packed, #z.gzip_decode(packed))`,
		SeeAlso: []string{"crypto", "io"},
		Entries: []Entry{
			{Name: "gzip_encode", Kind: EntryFunction, Signature: "compression.gzip_encode(s [, level]): string",
				Summary: "Compresses s in gzip format."},
			{Name: "gzip_decode", Kind: EntryFunction, Signature: "compression.gzip_decode(s): string",
				Summary: "Decompresses gzip data. Raises on corrupt input."},
			{Name: "zlib_encode", Kind: EntryFunction, Signature: "compression.zlib_encode(s [, level]): string",
				Summary: "Compresses s in zlib format."},
			{Name: "zlib_decode", Kind: EntryFunction, Signature: "compression.zlib_decode(s): string",
				Summary: "Decompresses zlib data. Raises on corrupt input."},
			{Name: "deflate_encode", Kind: EntryFunction, Signature: "compression.deflate_encode(s [, level]): string",
				Summary: "Compresses s as a raw DEFLATE stream, with no wrapper."},
			{Name: "deflate_decode", Kind: EntryFunction, Signature: "compression.deflate_decode(s): string",
				Summary: "Decompresses a raw DEFLATE stream."},
			{Name: "rle_encode", Kind: EntryFunction, Signature: "compression.rle_encode(s): string",
				Summary: "Run-length encodes s. A teaching implementation with its own format."},
			{Name: "rle_decode", Kind: EntryFunction, Signature: "compression.rle_decode(s): string",
				Summary: "Reverses rle_encode."},
			{Name: "encode", Kind: EntryFunction, Signature: "compression.encode(s): table",
				Summary: "Huffman-encodes s, returning a table with the bit string and the code table."},
			{Name: "decode", Kind: EntryFunction, Signature: "compression.decode(t): string",
				Summary: "Reverses compression.encode, taking the table it produced."},
			{Name: "codes", Kind: EntryFunction, Signature: "compression.codes(s): table",
				Summary: "The Huffman code assigned to each symbol of s."},
			{Name: "symbol_count", Kind: EntryFunction, Signature: "compression.symbol_count(s): table",
				Summary: "A frequency table of the symbols in s."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "compression.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "bit32", Kind: KindModule, RuntimeModule: "bit32",
		Title:    "32-bit bitwise operations",
		Synopsis: `local bit32 = require("bit32")`,
		Detail: `Arguments are truncated to unsigned 32-bit before the operation and
results come back in [0, 2^32). Shift counts outside 0..31 yield 0,
matching Lua 5.2's bit32.

luascript also has native bitwise operators (& | ~ << >> and the ~ unary)
that work on full 64-bit integers. Reach for bit32 when you specifically
need 32-bit wraparound.`,
		Example: `local bit32 = require("bit32")
print(bit32.band(0xF0, 0x3C))
print(bit32.extract(0xFF00, 8, 8))`,
		SeeAlso: []string{"math", "utf8"},
		Entries: []Entry{
			{Name: "band", Kind: EntryFunction, Signature: "bit32.band(...): number", Summary: "Bitwise AND of every argument."},
			{Name: "bor", Kind: EntryFunction, Signature: "bit32.bor(...): number", Summary: "Bitwise OR of every argument."},
			{Name: "bxor", Kind: EntryFunction, Signature: "bit32.bxor(...): number", Summary: "Bitwise exclusive OR of every argument."},
			{Name: "bnot", Kind: EntryFunction, Signature: "bit32.bnot(x): number", Summary: "Bitwise complement of x."},
			{Name: "btest", Kind: EntryFunction, Signature: "bit32.btest(...): boolean",
				Summary: "True when the bitwise AND of every argument is non-zero."},
			{Name: "lshift", Kind: EntryFunction, Signature: "bit32.lshift(x, n): number", Summary: "x shifted left n bits, filling with zeros."},
			{Name: "rshift", Kind: EntryFunction, Signature: "bit32.rshift(x, n): number", Summary: "x shifted right n bits, filling with zeros."},
			{Name: "arshift", Kind: EntryFunction, Signature: "bit32.arshift(x, n): number",
				Summary: "Arithmetic right shift: x shifted right n bits, replicating the sign bit."},
			{Name: "lrotate", Kind: EntryFunction, Signature: "bit32.lrotate(x, n): number", Summary: "x rotated left n bits."},
			{Name: "rrotate", Kind: EntryFunction, Signature: "bit32.rrotate(x, n): number", Summary: "x rotated right n bits."},
			{Name: "extract", Kind: EntryFunction, Signature: "bit32.extract(n, field [, width]): number",
				Summary: "The width bits (default 1) of n starting at bit field, counted from 0."},
			{Name: "replace", Kind: EntryFunction, Signature: "bit32.replace(n, v, field [, width]): number",
				Summary: "n with the width bits at position field replaced by the low bits of v."},
			{Name: "bswap", Kind: EntryFunction, Signature: "bit32.bswap(x): number",
				Summary: "x with its four bytes reversed — a 32-bit endianness swap."},
		},
	},
	{
		Name: "utf8", Kind: KindModule, RuntimeModule: "utf8",
		Title:    "UTF-8 aware string handling",
		Synopsis: `local utf8 = require("utf8")`,
		Detail: `luascript strings are byte strings; # and string.sub count bytes.
These functions interpret those bytes as UTF-8 so you can count and
index characters instead.`,
		Example: `local utf8 = require("utf8")
local s = "héllo"
print(#s, utf8.len(s))          --> 6  5
for p, c in utf8.codes(s) do print(p, c) end`,
		SeeAlso: []string{"string", "bit32"},
		Entries: []Entry{
			{Name: "len", Kind: EntryFunction, Signature: "utf8.len(s [, i [, j]]): number?, number?",
				Summary: "The number of codepoints in s[i..j], or nil plus the offset of the first invalid byte."},
			{Name: "char", Kind: EntryFunction, Signature: "utf8.char(...): string",
				Summary: "Builds a string from the given codepoints."},
			{Name: "codepoint", Kind: EntryFunction, Signature: "utf8.codepoint(s [, i [, j]]): ...number",
				Summary: "The codepoints of the characters starting in s[i..j], as multiple values."},
			{Name: "offset", Kind: EntryFunction, Signature: "utf8.offset(s, n [, i]): number?",
				Summary: "The byte position of the n-th character from position i. Negative n counts backwards."},
			{Name: "codes", Kind: EntryFunction, Signature: "utf8.codes(s): iterator",
				Summary: "Iterates the string, yielding each character's byte position and codepoint."},
			{Name: "charpattern", Kind: EntryConstant, Signature: "utf8.charpattern: string",
				Summary: "A Lua pattern matching exactly one UTF-8 sequence, for use with gmatch."},
		},
	},
	{
		Name: "log", Kind: KindModule, RuntimeModule: "log",
		Title:    "levelled logging",
		Synopsis: `local log = require("log")`,
		Detail: `Six levels, from trace up to fatal. Messages below the current level
are dropped; the default level is info. log.fatal writes its message and
then exits the process.

Output goes to stderr unless set_output redirects it to a file path, or
to the strings "stdout" or "stderr".`,
		Example: `local log = require("log")
log.set_level("debug")
log.set_prefix("worker")
log.info("started", 1, true)
log.debug("detail")`,
		SeeAlso: []string{"debug", "io", "os"},
		Entries: []Entry{
			{Name: "trace", Kind: EntryFunction, Signature: "log.trace(...)", Summary: "Logs at trace level, the most verbose."},
			{Name: "debug", Kind: EntryFunction, Signature: "log.debug(...)", Summary: "Logs at debug level."},
			{Name: "info", Kind: EntryFunction, Signature: "log.info(...)", Summary: "Logs at info level, the default threshold."},
			{Name: "warn", Kind: EntryFunction, Signature: "log.warn(...)", Summary: "Logs at warning level."},
			{Name: "error", Kind: EntryFunction, Signature: "log.error(...)", Summary: "Logs at error level. Does not raise."},
			{Name: "fatal", Kind: EntryFunction, Signature: "log.fatal(...)",
				Summary: "Logs at fatal level and terminates the process. Deferred blocks do not run."},
			{Name: "log", Kind: EntryFunction, Signature: "log.log(level, ...)",
				Summary: "Logs at an explicitly named level."},
			{Name: "set_level", Kind: EntryFunction, Signature: "log.set_level(level)",
				Summary: `Sets the threshold below which messages are dropped: "trace", "debug", "info", "warn", "error" or "fatal".`},
			{Name: "get_level", Kind: EntryFunction, Signature: "log.get_level(): string", Summary: "The current threshold."},
			{Name: "set_prefix", Kind: EntryFunction, Signature: "log.set_prefix(s)",
				Summary: "Sets a prefix printed before every message."},
			{Name: "set_output", Kind: EntryFunction, Signature: "log.set_output(dest)",
				Summary: `Redirects output to a file path, or to "stdout" or "stderr".`},
			{Name: "get_output", Kind: EntryFunction, Signature: "log.get_output(): string", Summary: "The current output destination."},
			{Name: "close", Kind: EntryFunction, Signature: "log.close()",
				Summary: "Closes the log file, if output was redirected to one."},
			{Name: "LEVELS", Kind: EntryField, Signature: "log.LEVELS: table", Summary: "The level names, in increasing order of severity."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "log.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "debug", Kind: KindModule, RuntimeModule: "debug",
		Title:    "stack introspection (partial)",
		Synopsis: `local debug = require("debug")`,
		Detail: `A deliberate subset of Lua's debug library. traceback and getinfo are
real; sethook and gethook are STUBS that accept a call and do nothing —
the VM has no hook mechanism. Nothing here can mutate locals or upvalues.

traceback renders the same frame walk the runtime prints for an uncaught
error: one "<source>:<line>: in function 'name'" line per Lua frame,
innermost first. getinfo(level) fills currentline from the live frame;
getinfo(f) on a bare function value still reports -1, since a function
that is not running has no current line.`,
		SeeAlso: []string{"log", "syntax"},
		Entries: []Entry{
			{Name: "traceback", Kind: EntryFunction, Signature: "debug.traceback([message [, level]]): string",
				Summary: "A stack traceback, with message prepended when given."},
			{Name: "getinfo", Kind: EntryFunction, Signature: "debug.getinfo(f): table",
				Summary: "Information about a function: what, source, short_src, currentline, name, namewhat, nparams and isvararg."},
			{Name: "sethook", Kind: EntryFunction, Signature: "debug.sethook(...)",
				Summary: "Accepted and ignored — a stub for compatibility. The VM has no hooks."},
			{Name: "gethook", Kind: EntryFunction, Signature: "debug.gethook(): nil",
				Summary: "Always returns nil — a stub for compatibility."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "debug.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "db", Kind: KindModule, RuntimeModule: "db",
		Title:    "SQL database access",
		Synopsis: `local db = require("db")`,
		Detail: `A thin wrapper over Go's database/sql. Four drivers are linked in
by default:

  postgres    PostgreSQL          $1, $2 placeholders
  mysql       MySQL / MariaDB     ? placeholders
  sqlserver   SQL Server / Azure  @p1, @p2 placeholders
  sqlite      SQLite (pure Go)    ? placeholders

db.drivers() lists what this binary actually has. Aliases resolve, so
"pg"/"postgresql", "mariadb" and "mssql" all work, and "sqlite" finds
whichever SQLite backend was built in. Building with
-tags luascript_sqlite_cgo swaps the pure-Go SQLite for mattn/go-sqlite3,
which registers as "sqlite3"; the name "sqlite" resolves to either.

Bind-parameter syntax is the one thing that stops the same SQL running
against two databases. The module reports it — db.placeholder(driver, n)
and conn:placeholder(n) — rather than rewriting your SQL, since parsing
SQL to do that is easy to get subtly wrong.

Always pass values as parameters rather than splicing them into the SQL
string — the driver escapes them and the query plan is reusable.

Values come back typed: a number column is a Lua number on every driver,
including MySQL, whose wire protocol returns raw bytes. DECIMAL and
NUMERIC stay strings on purpose — float64 cannot hold them exactly.
NULL is nil, and timestamps are RFC3339 strings.

SQLite in-memory databases (":memory:") are pinned to a single pooled
connection, because each SQLite in-memory connection is a separate
database and a pool would otherwise lose your tables.`,
		Example: `local db = require("db")
print(table.concat(db.drivers(), ", "))

-- No server needed: SQLite runs in-process.
local conn = db.open("sqlite", ":memory:")
conn:exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)")
local n, id = conn:exec("INSERT INTO users (name, age) VALUES (?, ?)", "ada", 36)

local rows = conn:query("SELECT id, name FROM users WHERE age > " .. conn:placeholder(1), 30)
for _, r in ipairs(rows) do print(r.id, r.name) end
conn:close()`,
		SeeAlso: []string{"db.conn", "json", "plugin"},
		Entries: []Entry{
			{Name: "open", Kind: EntryFunction, Signature: "db.open(driver, datasource): conn",
				Summary: `Opens a connection pool. Driver names and aliases resolve against what is compiled in.`,
				Detail:  "sql.Open is lazy, so db.open pings before returning — a bad DSN raises here rather than on the first query."},
			{Name: "drivers", Kind: EntryFunction, Signature: "db.drivers(): table",
				Summary: "An array of the database/sql driver names this binary can open."},
			{Name: "placeholder", Kind: EntryFunction, Signature: "db.placeholder(driver [, n]): string",
				Summary: `How a driver spells its nth bind parameter: "?", "$1" or "@p1". n defaults to 1.`,
				Detail:  "Answers for drivers that are not compiled in too, so a script can generate SQL for a database it is not connected to."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "db.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "plugin", Kind: KindModule, RuntimeModule: "plugin",
		Title:    "load Go packages at run time",
		Synopsis: `local plugin = require("plugin")`,
		Detail: `plugin declares Go packages and functions, generates a Go source file
re-exporting them, compiles it with ` + "`go build -buildmode=plugin`" + `,
opens the result and dispatches calls through reflection.

NOT AVAILABLE ON WINDOWS. Go supports plugins only on linux, darwin and
freebsd, and only with cgo. require("plugin") resolves everywhere, but
where it is unsupported plugin.supported is false and generate/open
raise. Check plugin.supported before using it.

Conversion is driven by the Go parameter type, so one Lua number
satisfies an int, a float64 or a time.Duration. Values with no Lua
counterpart (structs, pointers, interfaces) come back as GoValues whose
methods and exported fields are reachable by name.

generate compiles and loads native code: it is arbitrary code execution
by design. The spec is validated to turn typos into Lua errors, not as a
security boundary.`,
		Example: `local plugin = require("plugin")
if not plugin.supported then error(plugin.unsupported_reason()) end
local p = plugin.generate("strutil", {
  packages  = { { name = "strings" } },
  functions = { { pkg = "strings", name = "ToUpper", as = "Upper" } },
})
print(p.Upper("hi"))`,
		SeeAlso: []string{"db", "os"},
		Entries: []Entry{
			{Name: "generate", Kind: EntryFunction, Signature: "plugin.generate(name, spec): plugin",
				Summary: "Renders, compiles and opens a plugin exporting the requested packages and functions.",
				Detail: `spec.packages is a list of { name = "import/path", prefix = "_" }
entries; spec.functions a list of { pkg = "strings", name = "ToUpper",
as = "Upper" }. Builds are cached by the hash of the generated source, so
an unchanged spec loads instantly.`},
			{Name: "open", Kind: EntryFunction, Signature: "plugin.open(path): plugin",
				Summary: "Opens a prebuilt .so plugin."},
			{Name: "dir", Kind: EntryFunction, Signature: "plugin.dir(): string",
				Summary: "The directory holding generated plugin builds. $LUASCRIPT_PLUGIN_DIR relocates it."},
			{Name: "unsupported_reason", Kind: EntryFunction, Signature: "plugin.unsupported_reason(): string?",
				Summary: "Why plugins are unavailable on this build, or nil when they are available."},
			{Name: "supported", Kind: EntryField, Signature: "plugin.supported: boolean",
				Summary: "Whether this platform and build can load plugins. False on Windows, and on any build without cgo."},
		},
	},
	{
		Name: "ui", Kind: KindModule, RuntimeModule: "ui",
		Title:    "desktop GUI (Fyne)",
		Synopsis: `local ui = require("ui")`,
		Detail: `Off by default. The standard build compiles a headless stub:
require("ui") resolves, but constructing a widget raises. The real
Fyne backend needs -tags luascript_ui, which pulls in OpenGL through cgo
and therefore a C toolchain (MSYS2 MinGW-w64 on Windows).

  go run -tags luascript_ui ./cmd/luascript examples/31_ui_module.lsc

Because the widget surface only exists in the tagged build, it is not
documented here — see examples/31_ui_module.lsc.`,
		SeeAlso: []string{"plot", "httpserver"},
	},
	{
		Name: "test", Kind: KindModule, RuntimeModule: "test",
		Title:    "unit testing",
		Synopsis: `local t = require("test")`,
		Detail: `Tests run the moment they are declared — test(name, fn) calls fn
right there — so a test file is an ordinary chunk and nothing is
deferred to a collection phase.

  luascript test                     run every *_test.lsc under .
  luascript test -v examples/tests   report every test, not just failures
  luascript test -run "rounds"       filter by name (Lua pattern or substring)
  luascript test -list               name the tests without running them
  luascript test -failfast           stop at the first failure

Each file gets a fresh VM, so one file cannot leak globals into the
next. Within a file, tests run in declaration order on the one VM
goroutine.

describe(name, fn) nests a name scope; a test's full name is its
scopes and its own name joined with "/", which is what -run matches
against. before_each/after_each apply to the scope they are declared
in and every scope nested inside it.

Assertions raise on failure, carrying the source position of the
assertion call. Running a test file directly (luascript foo_test.lsc)
still executes its tests and prints a line each, but only the test
subcommand prints a summary.`,
		Example: `local t = require("test")

t.describe("math", function()
  t.before_each(function() collectgarbage() end)

  t.test("rounds down", function()
    t.assert_eq(math.floor(3.7), 3)
  end)

  t.test("rejects a bad call", function()
    t.assert_error(function() error("boom") end, "boom")
  end)

  t.skip("not written yet")
end)`,
		SeeAlso: []string{"debug", "os"},
		Entries: []Entry{
			{Name: "test", Kind: EntryFunction, Signature: "test.test(name, fn)",
				Summary: "Declares and immediately runs one test."},
			{Name: "it", Kind: EntryFunction, Signature: "test.it(name, fn)",
				Summary: "Alias for test.test, for suites that read better as `it(\"does x\")`."},
			{Name: "describe", Kind: EntryFunction, Signature: "test.describe(name, fn)",
				Summary: "Groups the tests declared in fn under a name scope; scopes nest.",
				Detail:  "An error raised by fn itself — rather than by a test inside it — is recorded against the scope, so one broken group does not abort the file."},
			{Name: "skip", Kind: EntryFunction, Signature: "test.skip(name [, fn])",
				Summary: "Records a test as skipped without running it. The body is optional and ignored."},
			{Name: "before_each", Kind: EntryFunction, Signature: "test.before_each(fn)",
				Summary: "Runs fn before every test in the current scope and any nested scope."},
			{Name: "after_each", Kind: EntryFunction, Signature: "test.after_each(fn)",
				Summary: "Runs fn after every test in the current scope, including when the test failed."},
			{Name: "assert_eq", Kind: EntryFunction, Signature: "test.assert_eq(got, want [, msg])",
				Summary: "Fails unless got == want, __eq metamethod included."},
			{Name: "assert_ne", Kind: EntryFunction, Signature: "test.assert_ne(got, unwanted [, msg])",
				Summary: "Fails when got == unwanted."},
			{Name: "assert_deep_eq", Kind: EntryFunction, Signature: "test.assert_deep_eq(got, want [, msg])",
				Summary: "Compares tables key by key, recursively; cycles terminate.",
				Detail:  "A table whose metatable defines __eq is compared with that metamethod instead, so deep equality never contradicts ==."},
			{Name: "assert_true", Kind: EntryFunction, Signature: "test.assert_true(v [, msg])",
				Summary: "Fails unless v is truthy — only nil and false are not."},
			{Name: "assert_false", Kind: EntryFunction, Signature: "test.assert_false(v [, msg])",
				Summary: "Fails unless v is nil or false."},
			{Name: "assert_nil", Kind: EntryFunction, Signature: "test.assert_nil(v [, msg])",
				Summary: "Fails unless v is nil."},
			{Name: "assert_not_nil", Kind: EntryFunction, Signature: "test.assert_not_nil(v [, msg])",
				Summary: "Fails when v is nil."},
			{Name: "assert_near", Kind: EntryFunction, Signature: "test.assert_near(got, want [, eps] [, msg])",
				Summary: "Fails unless |got - want| <= eps. eps defaults to 1e-9.",
				Detail:  "A string in the third position is taken as the message, so the common (got, want, msg) form needs no placeholder tolerance."},
			{Name: "assert_type", Kind: EntryFunction, Signature: "test.assert_type(v, typename [, msg])",
				Summary: "Fails unless type(v) == typename."},
			{Name: "assert_len", Kind: EntryFunction, Signature: "test.assert_len(v, n [, msg])",
				Summary: "Fails unless #v == n. v must be a string or a table."},
			{Name: "assert_contains", Kind: EntryFunction, Signature: "test.assert_contains(haystack, needle [, msg])",
				Summary: "Substring search for a string haystack, value membership for a table."},
			{Name: "assert_match", Kind: EntryFunction, Signature: "test.assert_match(s, pattern [, msg])",
				Summary: "Fails unless the Lua pattern matches somewhere in s."},
			{Name: "assert_error", Kind: EntryFunction, Signature: "test.assert_error(fn [, pattern] [, msg]): any",
				Summary: "Fails unless fn raises; returns the error value. pattern is matched against its message."},
			{Name: "assert_no_error", Kind: EntryFunction, Signature: "test.assert_no_error(fn [, msg]): ...",
				Summary: "Fails if fn raises; forwards fn's return values."},
			{Name: "fail", Kind: EntryFunction, Signature: "test.fail([msg])",
				Summary: "Fails unconditionally — for a branch that should be unreachable."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "test.VERSION: string",
				Summary: "The module's version string."},
		},
	},
}
