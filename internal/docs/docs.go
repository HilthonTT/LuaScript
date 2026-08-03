// Package docs is the single source of truth for luascript's reference
// documentation: one curated entry per built-in global, library function,
// native module member, and object method.
//
// It is deliberately data-only (no VM, no compiler imports) so every
// consumer can share it:
//
//   - `luascript doc` renders topics as man pages (cmd/luascript/doc.go)
//   - the REPL's `doc <topic>` command prints the same pages
//   - the LSP server derives completion + hover from it (internal/lsp/server)
//
// The data is hand-maintained; it is NOT reflected out of the VM, because
// summaries and signatures cannot be recovered from a *vm.GoFunc. What the
// runtime *can* check is drift — cmd/luascript's TestDocsMatchRuntime loads
// every native module and fails when a documented member no longer exists,
// and `luascript doc -audit` reports members that exist but are undocumented.
package docs

import (
	"sort"
	"strings"
)

// Kind classifies a topic, which drives both the index grouping and the
// man-page header ("MODULE" vs "OBJECT" and so on).
type Kind string

const (
	// KindCore is the base library: globals installed into _G with no
	// namespace of their own (print, pcall, setmetatable, ...).
	KindCore Kind = "core"
	// KindLibrary is an auto-global namespace — available without a
	// require (string, table, math, coroutine, io).
	KindLibrary Kind = "library"
	// KindModule is a native module reached through require().
	KindModule Kind = "module"
	// KindObject is a value returned by a constructor, documented for its
	// methods (a file handle, a compiled regex, a figure, ...).
	KindObject Kind = "object"
)

// EntryKind separates callables from data so the renderer can group them
// under FUNCTIONS / METHODS and CONSTANTS / FIELDS.
type EntryKind string

const (
	EntryFunction EntryKind = "function"
	EntryMethod   EntryKind = "method"
	EntryConstant EntryKind = "constant"
	EntryField    EntryKind = "field"
	// EntryKeyword is a syntactic construct rather than a value — it has
	// no runtime member to check against, so the drift audit skips it.
	EntryKeyword EntryKind = "keyword"
)

// Entry is one documented name inside a topic.
type Entry struct {
	// Name is the bare member name as it appears at runtime ("floor").
	// It must match the key the VM actually installs — the drift test
	// compares these against a live module table.
	Name string
	// Signature is the qualified call form ("math.floor(x): number").
	Signature string
	// Summary is a single sentence, shown in indexes and search results.
	Summary string
	// Detail is optional extra prose shown only on the full page.
	Detail string
	Kind   EntryKind
}

// Topic is one man page: a namespace and everything reachable through it.
type Topic struct {
	// Name is the lookup key and the page title ("math", "std.stack").
	Name string
	Kind Kind
	// Aliases are alternative lookup keys ("globals" for "_G").
	Aliases []string
	// Title is the short "name — title" line under NAME.
	Title string
	// Synopsis is how you get hold of the namespace, verbatim luascript.
	Synopsis string
	// Detail is the DESCRIPTION body. Blank lines separate paragraphs;
	// lines starting with two spaces are kept verbatim (code samples).
	Detail  string
	Entries []Entry
	// Example is a runnable snippet, printed verbatim under EXAMPLES.
	Example string
	// SeeAlso lists related topic names.
	SeeAlso []string
	// RuntimeModule is the require() name whose live table backs this
	// topic, when there is one. Only set for KindModule and for objects
	// reachable from a module; the drift audit uses it.
	RuntimeModule string
	// RuntimeGlobal is the auto-global namespace backing this topic —
	// "string", "math", ... or "_G" for the globals table itself. A topic
	// may set both this and RuntimeModule (math and io do), in which case
	// the audit checks the union of the two surfaces.
	RuntimeGlobal string
	// Requireable marks an auto-global namespace that ALSO has a native
	// module of the same name with a larger surface (math and io). Such a
	// topic documents the union and is listed under both index headings;
	// entries that exist on only one of the two say so in their Detail.
	Requireable bool
}

// registry is every topic, assembled once from the data_*.go files.
var registry = build()

func build() []Topic {
	var all []Topic
	all = append(all, coreTopics...)
	all = append(all, libraryTopics...)
	all = append(all, moduleTopics...)
	all = append(all, datascienceTopics...)
	all = append(all, objectTopics...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}

// index maps every lookup key (name + aliases) to its topic.
var index = buildIndex()

func buildIndex() map[string]*Topic {
	m := make(map[string]*Topic, len(registry)*2)
	for i := range registry {
		t := &registry[i]
		m[t.Name] = t
		for _, a := range t.Aliases {
			m[a] = t
		}
	}
	return m
}

// All returns every topic, sorted by name.
func All() []Topic { return registry }

// Topics returns the topics of one kind, sorted by name. A Requireable
// library (math, io) is reported under KindModule as well, since that is
// where someone hunting for a require target will look for it.
func Topics(k Kind) []Topic {
	var out []Topic
	for _, t := range registry {
		if t.Kind == k || (k == KindModule && t.Requireable) {
			out = append(out, t)
		}
	}
	return out
}

// LookupTopic resolves a topic by name or alias.
func LookupTopic(name string) (*Topic, bool) {
	t, ok := index[name]
	return t, ok
}

// LookupEntry resolves "ns.member" (or "ns:member" for methods) to the entry
// and its owning topic.
func LookupEntry(qualified string) (*Topic, *Entry, bool) {
	ns, member, ok := splitQualified(qualified)
	if !ok {
		return nil, nil, false
	}
	return lookupIn(ns, member)
}

// Lookup resolves a free-form query the way the CLI wants it: a topic name
// wins over a member of the same spelling, then "ns.member" / "ns:member".
// It reports the topic, the entry (nil when the whole topic matched), and
// whether anything was found.
func Lookup(query string) (*Topic, *Entry, bool) {
	if t, ok := LookupTopic(query); ok {
		return t, nil, true
	}
	if t, e, ok := LookupEntry(query); ok {
		return t, e, true
	}
	// A bare member name is unambiguous often enough to be worth resolving
	// (`luascript doc gsub`), but only when exactly one topic defines it.
	var (
		hitT *Topic
		hitE *Entry
		n    int
	)
	for i := range registry {
		for j := range registry[i].Entries {
			if registry[i].Entries[j].Name == query {
				hitT, hitE = &registry[i], &registry[i].Entries[j]
				n++
			}
		}
	}
	if n == 1 {
		return hitT, hitE, true
	}
	return nil, nil, false
}

// LookupMember resolves a namespace + member pair ("math", "floor").
func LookupMember(ns, member string) (*Topic, *Entry, bool) {
	return lookupIn(ns, member)
}

func lookupIn(ns, member string) (*Topic, *Entry, bool) {
	t, ok := index[ns]
	if !ok {
		return nil, nil, false
	}
	for i := range t.Entries {
		if t.Entries[i].Name == member {
			return t, &t.Entries[i], true
		}
	}
	return nil, nil, false
}

// splitQualified splits "a.b" or "a:b" on the LAST separator, so object
// topics whose names are themselves dotted ("std.stack:push") resolve.
func splitQualified(s string) (ns, member string, ok bool) {
	i := strings.LastIndexAny(s, ".:")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// Result is one hit from Search.
type Result struct {
	Topic *Topic
	// Entry is nil when the topic itself matched.
	Entry *Entry
}

// Name renders the result's fully qualified name.
func (r Result) Name() string {
	if r.Entry == nil {
		return r.Topic.Name
	}
	return r.Topic.Name + "." + r.Entry.Name
}

// Summary renders the result's one-line description.
func (r Result) Summary() string {
	if r.Entry == nil {
		return r.Topic.Title
	}
	return r.Entry.Summary
}

// Search is apropos: a case-insensitive substring match over topic names,
// titles, member names, signatures, and summaries. Results are ordered
// topics-first, then by qualified name.
func Search(q string) []Result {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var out []Result
	for i := range registry {
		t := &registry[i]
		if contains(t.Name, q) || contains(t.Title, q) || contains(t.Detail, q) {
			out = append(out, Result{Topic: t})
		}
		for j := range t.Entries {
			e := &t.Entries[j]
			if contains(e.Name, q) || contains(e.Signature, q) || contains(e.Summary, q) {
				out = append(out, Result{Topic: t, Entry: e})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Entry == nil) != (out[j].Entry == nil) {
			return out[i].Entry == nil
		}
		return out[i].Name() < out[j].Name()
	})
	return out
}

func contains(haystack, lowerNeedle string) bool {
	return strings.Contains(strings.ToLower(haystack), lowerNeedle)
}

// Functions returns the topic's callable entries, Keywords its syntactic
// ones, and Constants the rest. All three preserve declaration order, which
// the data files keep meaningful (constructors first, then operations).
func (t *Topic) Functions() []Entry { return t.entriesOfKind(EntryFunction, EntryMethod) }

// Keywords returns the topic's syntactic constructs.
func (t *Topic) Keywords() []Entry { return t.entriesOfKind(EntryKeyword) }

// Constants returns the topic's non-callable entries.
func (t *Topic) Constants() []Entry { return t.entriesOfKind(EntryConstant, EntryField) }

func (t *Topic) entriesOfKind(kinds ...EntryKind) []Entry {
	var out []Entry
	for _, e := range t.Entries {
		for _, k := range kinds {
			if e.Kind == k {
				out = append(out, e)
				break
			}
		}
	}
	return out
}
