package docs

import (
	"strings"
	"testing"
)

func TestLookupTopic(t *testing.T) {
	for _, name := range []string{"math", "string", "queue", "std.stack", "_G", "syntax"} {
		if _, ok := LookupTopic(name); !ok {
			t.Errorf("LookupTopic(%q) not found", name)
		}
	}
	if _, ok := LookupTopic("nope"); ok {
		t.Error("LookupTopic(\"nope\") should not resolve")
	}
}

func TestLookupTopicAlias(t *testing.T) {
	got, ok := LookupTopic("globals")
	if !ok || got.Name != "_G" {
		t.Fatalf(`LookupTopic("globals") = %v, %v; want the _G topic`, got, ok)
	}
}

func TestLookupEntry(t *testing.T) {
	topic, entry, ok := LookupEntry("math.floor")
	if !ok {
		t.Fatal("math.floor not found")
	}
	if topic.Name != "math" || entry.Name != "floor" {
		t.Errorf("got %s.%s, want math.floor", topic.Name, entry.Name)
	}
	// Colon form, for methods.
	if _, _, ok := LookupEntry("std.stack:push"); !ok {
		t.Error("std.stack:push not found via the colon form")
	}
	// An object topic's own name contains a dot; splitting must happen on
	// the LAST separator, not the first.
	if _, _, ok := LookupEntry("plot.figure.save"); !ok {
		t.Error("plot.figure.save not found — qualified split is wrong")
	}
}

func TestLookupPrefersTopicOverMember(t *testing.T) {
	// "sort" is both a module and a member of table. The topic wins.
	topic, entry, ok := Lookup("sort")
	if !ok || entry != nil || topic.Name != "sort" {
		t.Errorf("Lookup(\"sort\") = %v, %v; want the sort topic", topic, entry)
	}
}

func TestLookupBareMemberWhenUnambiguous(t *testing.T) {
	topic, entry, ok := Lookup("gsub")
	if !ok || entry == nil {
		t.Fatalf(`Lookup("gsub") = %v, %v, %v`, topic, entry, ok)
	}
	if topic.Name != "string" || entry.Name != "gsub" {
		t.Errorf("got %s.%s, want string.gsub", topic.Name, entry.Name)
	}
	// "insert" is defined by table, std.trie and std.btree, so a bare
	// lookup is ambiguous and must not guess.
	if _, _, ok := Lookup("insert"); ok {
		t.Error(`Lookup("insert") resolved despite being ambiguous`)
	}
}

func TestSearch(t *testing.T) {
	results := Search("hmac")
	if len(results) == 0 {
		t.Fatal("Search(\"hmac\") found nothing")
	}
	var names []string
	for _, r := range results {
		names = append(names, r.Name())
	}
	joined := strings.Join(names, " ")
	if !strings.Contains(joined, "crypto.hmac_sha256") {
		t.Errorf("Search(\"hmac\") = %v; want crypto.hmac_sha256 among them", names)
	}
	if len(Search("")) != 0 {
		t.Error("an empty query should match nothing")
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	if len(Search("JSON")) == 0 {
		t.Error(`Search("JSON") found nothing`)
	}
}

func TestRenderTopic(t *testing.T) {
	topic, _ := LookupTopic("json")
	out := RenderTopic(topic, Options{Width: 80})
	for _, want := range []string{"NAME", "SYNOPSIS", "DESCRIPTION", "FUNCTIONS",
		"json.encode(value [, opts]): string", "SEE ALSO", "JSON(3)"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderTopic(json) missing %q\n%s", want, out)
		}
	}
	assertWidth(t, out, 80)
}

func TestRenderTopicSectionsByKind(t *testing.T) {
	obj, _ := LookupTopic("std.stack")
	if out := RenderTopic(obj, Options{}); !strings.Contains(out, "METHODS") {
		t.Errorf("an object topic should render METHODS, got:\n%s", out)
	}
	syn, _ := LookupTopic("syntax")
	if out := RenderTopic(syn, Options{}); !strings.Contains(out, "CONSTRUCTS") {
		t.Errorf("the syntax topic should render CONSTRUCTS, got:\n%s", out)
	}
}

func TestRenderEntry(t *testing.T) {
	topic, entry, _ := LookupEntry("string.gsub")
	out := RenderEntry(topic, entry, Options{Width: 72})
	if !strings.Contains(out, "string.gsub(s, pat, repl [, n]): string, number") {
		t.Errorf("RenderEntry missing the signature:\n%s", out)
	}
	assertWidth(t, out, 72)
}

func TestRenderIndexListsEveryKind(t *testing.T) {
	out := RenderIndex(Options{Width: 100})
	for _, want := range []string{"BASE LIBRARY", "AUTO-GLOBAL LIBRARIES",
		"MODULES (require)", "OBJECTS", "ndarray", "std.trie"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderIndex missing %q", want)
		}
	}
	assertWidth(t, out, 100)
}

// A Requireable library belongs under both index headings: it is an
// auto-global AND a require target.
func TestRequireableAppearsUnderModules(t *testing.T) {
	var found bool
	for _, top := range Topics(KindModule) {
		if top.Name == "math" {
			found = true
		}
	}
	if !found {
		t.Error("math should be listed among the require-able modules")
	}
}

func TestSuggest(t *testing.T) {
	got := Suggest("mat")
	if len(got) == 0 || got[0] != "math" {
		t.Errorf(`Suggest("mat") = %v; want math first`, got)
	}
	if len(Suggest("zzzz")) != 0 {
		t.Error("a query matching nothing should suggest nothing")
	}
}

func TestColorOnlyWhenRequested(t *testing.T) {
	topic, _ := LookupTopic("uuid")
	if strings.Contains(RenderTopic(topic, Options{}), "\033[") {
		t.Error("rendered ANSI escapes with Color disabled")
	}
	if !strings.Contains(RenderTopic(topic, Options{Color: true}), "\033[") {
		t.Error("rendered no ANSI escapes with Color enabled")
	}
}

// Every topic must carry the fields the renderer relies on, and every
// SeeAlso must point at a topic that exists — a dangling cross-reference is
// a dead end for the reader.
func TestTopicsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, top := range All() {
		if top.Name == "" || top.Title == "" {
			t.Errorf("topic %q is missing a name or title", top.Name)
		}
		if seen[top.Name] {
			t.Errorf("duplicate topic %q", top.Name)
		}
		seen[top.Name] = true

		for _, ref := range top.SeeAlso {
			if _, ok := LookupTopic(ref); !ok {
				t.Errorf("topic %q: SEE ALSO points at unknown topic %q", top.Name, ref)
			}
		}
		names := map[string]bool{}
		for _, e := range top.Entries {
			switch {
			case e.Name == "":
				t.Errorf("topic %q has an entry with no name", top.Name)
			case e.Signature == "":
				t.Errorf("%s.%s has no signature", top.Name, e.Name)
			case e.Summary == "":
				t.Errorf("%s.%s has no summary", top.Name, e.Name)
			case e.Kind == "":
				t.Errorf("%s.%s has no kind", top.Name, e.Name)
			}
			if names[e.Name] {
				t.Errorf("topic %q lists %q twice", top.Name, e.Name)
			}
			names[e.Name] = true
		}
	}
}

// Entry signatures should start with the qualified name, so a reader who
// copies one out of a search result gets something they can actually call.
func TestSignaturesAreQualified(t *testing.T) {
	for _, top := range All() {
		if top.Kind == KindObject || top.Name == "_G" || top.Name == "syntax" {
			continue // receiver-style and bare-word signatures
		}
		for _, e := range top.Entries {
			if !strings.HasPrefix(e.Signature, top.Name+"."+e.Name) {
				t.Errorf("%s.%s: signature %q should start with %q",
					top.Name, e.Name, e.Signature, top.Name+"."+e.Name)
			}
		}
	}
}

// assertWidth checks that nothing overflows the requested column count.
// Verbatim blocks (examples, synopses) are exempt: they are copied through
// unwrapped on purpose.
func assertWidth(t *testing.T, out string, width int) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "local ") ||
			strings.Contains(line, "require(") {
			continue
		}
		if n := len([]rune(line)); n > width {
			t.Errorf("line exceeds width %d (%d): %q", width, n, line)
		}
	}
}
