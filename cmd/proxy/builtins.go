package proxy

import "github.com/hilthontt/luascript/lsp/protocol"

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

// hoverDocs is the label -> markdown lookup used by textDocument/hover. It is
// assembled once from the same tables that feed completion so the two never
// drift apart.
var hoverDocs = buildHoverDocs()

func buildHoverDocs() map[string]string {
	m := make(map[string]string, len(globals)+len(modules)+len(keywords))
	for _, b := range globals {
		m[b.label] = "```luascript\n" + b.detail + "\n```\n\n" + b.doc
	}
	for _, b := range modules {
		if _, exists := m[b.label]; exists {
			continue
		}
		m[b.label] = "```luascript\n" + b.detail + "\n```\n\n" + b.doc
	}
	for _, kw := range keywords {
		if _, exists := m[kw]; !exists {
			m[kw] = "`" + kw + "` — luascript keyword."
		}
	}
	return m
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
