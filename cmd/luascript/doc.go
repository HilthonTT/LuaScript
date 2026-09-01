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

func auditDocs() (undocumented, missing []string) {
	for _, topic := range docs.All() {
		live, ok := liveMembers(topic)
		if !ok {
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

func skipInAudit(name string) bool {
	if strings.HasPrefix(name, "__") {
		return true
	}
	if _, isTopic := docs.LookupTopic(name); isTopic {
		return true
	}
	return false
}

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
