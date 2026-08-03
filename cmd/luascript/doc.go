package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hilthontt/luascript/internal/docs"
	"github.com/hilthontt/luascript/internal/vm"
)

// runDoc implements `luascript doc` — a man page for the standard library.
//
//	luascript doc                    index of every topic
//	luascript doc math               the math page
//	luascript doc math.floor         one entry
//	luascript doc string format      the same, as two words
//	luascript doc -k random          apropos: search names and summaries
//	luascript doc -all               every page, for piping to a file
//	luascript doc -audit             report runtime members with no docs
//
// Exit codes: 0 success, 1 nothing found, 2 usage error.
func runDoc(argv []string) int {
	fs := flag.NewFlagSet("doc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	search := fs.String("k", "", "search topic names, signatures and summaries (apropos)")
	all := fs.Bool("all", false, "print every page")
	list := fs.Bool("list", false, "print topic names only, one per line")
	audit := fs.Bool("audit", false, "compare the docs against the live runtime and report gaps")
	width := fs.Int("width", 0, "line width (default 80, or $COLUMNS)")
	noColor := fs.Bool("no-color", false, "disable ANSI styling")
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	opts := docs.Options{
		Width: resolveWidth(*width),
		Color: !*noColor && colorAllowed(os.Stdout),
	}
	out := os.Stdout

	switch {
	case *audit:
		return runDocAudit(out)
	case *list:
		for _, name := range docs.TopicNames() {
			fmt.Fprintln(out, name)
		}
		return 0
	case *all:
		io.WriteString(out, docs.RenderAll(opts))
		return 0
	case *search != "":
		results := docs.Search(*search)
		if len(results) == 0 {
			fmt.Fprintf(os.Stderr, "doc: nothing matches %q\n", *search)
			return 1
		}
		io.WriteString(out, docs.RenderSearch(results, opts))
		return 0
	}

	args := fs.Args()
	if len(args) == 0 {
		io.WriteString(out, docs.RenderIndex(opts))
		return 0
	}

	// `doc string format` is the same request as `doc string.format`; join
	// the words so both spellings hit one lookup.
	query := args[0]
	if len(args) > 1 {
		query = strings.Join(args[:2], ".")
	}

	topic, entry, ok := docs.Lookup(query)
	if !ok {
		fmt.Fprintf(os.Stderr, "doc: no documentation for %q\n", query)
		if sugg := docs.Suggest(query); len(sugg) > 0 {
			fmt.Fprintf(os.Stderr, "did you mean: %s\n", strings.Join(sugg, ", "))
		}
		fmt.Fprintln(os.Stderr, `run "luascript doc" for the index`)
		return 1
	}
	if entry != nil {
		io.WriteString(out, docs.RenderEntry(topic, entry, opts))
		return 0
	}
	io.WriteString(out, docs.RenderTopic(topic, opts))
	return 0
}

// resolveWidth picks the render width: an explicit flag wins, then
// $COLUMNS, then 80. Terminal size is not queried — that needs a syscall
// per platform, and $COLUMNS covers the case where someone cares.
func resolveWidth(flagWidth int) int {
	if flagWidth > 0 {
		return flagWidth
	}
	if c := os.Getenv("COLUMNS"); c != "" {
		n := 0
		if _, err := fmt.Sscanf(c, "%d", &n); err == nil && n >= 40 {
			return n
		}
	}
	return 80
}

// colorAllowed reports whether ANSI styling should be emitted: only to a
// terminal, and never when NO_COLOR is set (https://no-color.org).
func colorAllowed(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// --- audit ------------------------------------------------------------

// runDocAudit loads every native module in a throwaway VM and compares the
// live member tables against the docs registry. It reports members that
// exist but are undocumented, and documented members that no longer exist.
//
// This lives in package main rather than in internal/docs because it needs
// nativeRegistrars, the single source of truth for which modules ship.
func runDocAudit(out *os.File) int {
	undocumented, missing := auditDocs()

	if len(undocumented) > 0 {
		fmt.Fprintf(out, "undocumented (present at runtime, absent from internal/docs):\n")
		for _, name := range undocumented {
			fmt.Fprintf(out, "  %s\n", name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(out, "stale (documented, absent at runtime):\n")
		for _, name := range missing {
			fmt.Fprintf(out, "  %s\n", name)
		}
	}
	if len(undocumented) == 0 && len(missing) == 0 {
		fmt.Fprintln(out, "docs are in sync with the runtime")
		return 0
	}
	fmt.Fprintf(out, "\n%d undocumented, %d stale\n", len(undocumented), len(missing))
	return 1
}

// auditDocs is the shared body of `doc -audit` and TestDocsMatchRuntime.
// Both lists are sorted and qualified as "module.member".
func auditDocs() (undocumented, missing []string) {
	for _, topic := range docs.All() {
		live, ok := liveMembers(topic)
		if !ok {
			// Nothing to compare against: the topic names no runtime
			// surface, or the module refused to load in a headless VM
			// (the ui stub is the standing example).
			continue
		}
		documented := make(map[string]bool, len(topic.Entries))
		for _, e := range topic.Entries {
			if e.Kind == docs.EntryKeyword {
				continue
			}
			documented[e.Name] = true
			if !live[e.Name] && !isObjectTopic(topic) {
				missing = append(missing, topic.Name+"."+e.Name)
			}
		}
		// Only module-level topics own the module's namespace; an object
		// topic documents methods that live on a constructed value, which
		// this reflection cannot reach.
		if isObjectTopic(topic) {
			continue
		}
		for name := range live {
			if documented[name] || skipInAudit(name) {
				continue
			}
			undocumented = append(undocumented, topic.Name+"."+name)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(missing)
	return undocumented, missing
}

func isObjectTopic(t docs.Topic) bool { return t.Kind == docs.KindObject }

// skipInAudit filters names that are real but deliberately not documented
// under the topic that owns them: the compiler's lowering helpers
// (__enum_freeze and friends), and the auto-global namespaces, which have
// pages of their own rather than an entry inside _G.
func skipInAudit(name string) bool {
	if strings.HasPrefix(name, "__") {
		return true
	}
	if _, isTopic := docs.LookupTopic(name); isTopic {
		return true
	}
	return false
}

// liveMembers gathers the runtime surface a topic claims: its native module,
// its auto-global namespace, or the union when it has both (math and io).
func liveMembers(t docs.Topic) (map[string]bool, bool) {
	out := map[string]bool{}
	found := false
	if t.RuntimeModule != "" {
		if m, ok := loadModuleMembers(t.RuntimeModule); ok {
			found = true
			for k := range m {
				out[k] = true
			}
		}
	}
	if t.RuntimeGlobal != "" {
		if m, ok := loadGlobalMembers(t.RuntimeGlobal); ok {
			found = true
			for k := range m {
				out[k] = true
			}
		}
	}
	return out, found
}

// loadModuleMembers runs a module's preload loader in a fresh VM and returns
// the set of string keys reachable on the table it hands back.
func loadModuleMembers(name string) (map[string]bool, bool) {
	v := vm.New()
	registerAllNatives(v)

	pkg, ok := v.Globals.Get("package").(*vm.Table)
	if !ok {
		return nil, false
	}
	preload, ok := pkg.Get("preload").(*vm.Table)
	if !ok {
		return nil, false
	}
	loader := preload.Get(name)
	if loader == nil {
		return nil, false
	}
	res, _, failed := v.SafeCall(loader, []vm.Value{name})
	if failed || len(res) == 0 {
		return nil, false
	}
	mod, ok := res[0].(*vm.Table)
	if !ok {
		return nil, false
	}
	return tableMembers(mod), true
}

// loadGlobalMembers returns the members of an auto-global namespace, or of
// the globals table itself for the name "_G".
func loadGlobalMembers(name string) (map[string]bool, bool) {
	v := vm.New()
	registerAllNatives(v)

	if name == "_G" {
		return tableMembers(v.Globals), true
	}
	ns, ok := v.Globals.Get(name).(*vm.Table)
	if !ok {
		return nil, false
	}
	return tableMembers(ns), true
}

// tableMembers collects a table's string keys, following the metatable's
// __index (most native modules keep their functions there) and skipping the
// private instance keys native objects hide under a NUL prefix.
func tableMembers(t *vm.Table) map[string]bool {
	members := map[string]bool{}
	collect := func(tb *vm.Table) {
		var k vm.Value
		for {
			k, _ = tb.Next(k)
			if k == nil {
				return
			}
			if s, ok := k.(string); ok && !strings.HasPrefix(s, "\x00") {
				members[s] = true
			}
		}
	}
	collect(t)
	if mt := t.Metatable(); mt != nil {
		if idx, ok := mt.Get("__index").(*vm.Table); ok {
			collect(idx)
		}
	}
	return members
}
