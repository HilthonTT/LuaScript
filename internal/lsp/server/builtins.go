package server

import (
	"github.com/hilthontt/luascript/internal/docs"
	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

// The completion and hover data is derived from internal/docs, the same
// curated registry `luascript doc` renders as man pages. Nothing is
// hand-maintained here: adding a stdlib entry there surfaces it in the
// editor automatically, and `luascript doc -audit` keeps that registry
// honest against the live VM.

// builtin describes one completable / hoverable identifier.
type builtin struct {
	label  string
	kind   protocol.CompletionItemKind
	detail string
	doc    string
}

// keywords are the Lua 5.4 + luascript reserved words (compiler/token).
// This is the lexer's list rather than the docs registry's, because
// completion wants every reserved word — including the ones that only make
// sense as part of a larger construct (then, end, until, ...) and so have no
// page of their own.
var keywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
	"match", "enum", "defer", "try", "catch", "throw",
	"continue", "struct", "type",
}

// globals are the builtin global functions installed by vm/stdlib.go.
var globals = builtinsFromTopic("_G")

// modules are the namespaces reachable through require(), plus the
// auto-global libraries — both are worth completing at top level.
var modules = buildModuleBuiltins()

// members maps a namespace to the fields reachable through it via `ns.field`,
// feeding dotted completion (`math.` -> floor, ceil, ...) and qualified hover.
var members = buildMembers()

func builtinsFromTopic(name string) []builtin {
	t, ok := docs.LookupTopic(name)
	if !ok {
		return nil
	}
	out := make([]builtin, 0, len(t.Entries))
	for _, e := range t.Entries {
		out = append(out, builtin{
			label:  e.Name,
			kind:   completionKind(e.Kind),
			detail: e.Signature,
			doc:    entryDoc(e),
		})
	}
	return out
}

func buildModuleBuiltins() []builtin {
	var out []builtin
	seen := map[string]bool{}
	add := func(ts []docs.Topic, synopsis func(docs.Topic) string) {
		for _, t := range ts {
			if seen[t.Name] {
				continue
			}
			seen[t.Name] = true
			out = append(out, builtin{
				label:  t.Name,
				kind:   protocol.CompletionItemKindModule,
				detail: synopsis(t),
				doc:    t.Title,
			})
		}
	}
	add(docs.Topics(docs.KindModule), func(t docs.Topic) string { return `require("` + t.Name + `")` })
	add(docs.Topics(docs.KindLibrary), func(t docs.Topic) string { return t.Name })
	return out
}

func buildMembers() map[string][]builtin {
	m := map[string][]builtin{}
	for _, t := range docs.All() {
		if t.Kind == docs.KindCore || len(t.Entries) == 0 {
			continue
		}
		m[t.Name] = builtinsFromTopic(t.Name)
	}
	return m
}

func completionKind(k docs.EntryKind) protocol.CompletionItemKind {
	switch k {
	case docs.EntryConstant:
		return protocol.CompletionItemKindConstant
	case docs.EntryField:
		return protocol.CompletionItemKindField
	case docs.EntryKeyword:
		return protocol.CompletionItemKindKeyword
	default:
		return protocol.CompletionItemKindFunction
	}
}

// entryDoc is the markdown body: the summary, plus the long-form detail as
// a second paragraph when there is one.
func entryDoc(e docs.Entry) string {
	if e.Detail == "" {
		return e.Summary
	}
	return e.Summary + "\n\n" + e.Detail
}

// hoverDocs is the label -> markdown lookup used by textDocument/hover. Keys
// are bare names (`print`, `math`) and qualified member names (`math.floor`)
// so hover works on either half of a dotted expression.
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
	// Keyword hovers come from the syntax page where it documents one, so
	// `if` explains the if *expression* too. The rest get a bare label.
	for _, kw := range keywords {
		if _, exists := m[kw]; exists {
			continue
		}
		if _, e, ok := docs.LookupMember("syntax", kw); ok {
			m[kw] = render(builtin{detail: e.Signature, doc: entryDoc(*e)})
			continue
		}
		m[kw] = "`" + kw + "` — luascript keyword."
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
// namespace names.
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
