package server

import (
	"github.com/hilthontt/luascript/internal/docs"
	"github.com/hilthontt/luascript/internal/lsp/protocol"
)

type builtin struct {
	label  string
	kind   protocol.CompletionItemKind
	detail string
	doc    string
}

var keywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
	"match", "enum", "defer", "try", "catch", "throw",
	"continue", "struct", "type",
}

var globals = builtinsFromTopic("_G")

var modules = buildModuleBuiltins()

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

func entryDoc(e docs.Entry) string {
	if e.Detail == "" {
		return e.Summary
	}
	return e.Summary + "\n\n" + e.Detail
}

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
