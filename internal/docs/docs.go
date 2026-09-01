package docs

import (
	"sort"
	"strings"
)

type Kind string

const (
	KindCore    Kind = "core"
	KindLibrary Kind = "library"
	KindModule  Kind = "module"
	KindObject  Kind = "object"
)

type EntryKind string

const (
	EntryFunction EntryKind = "function"
	EntryMethod   EntryKind = "method"
	EntryConstant EntryKind = "constant"
	EntryField    EntryKind = "field"
	EntryKeyword  EntryKind = "keyword"
)

type Entry struct {
	Name      string
	Signature string
	Summary   string
	Detail    string
	Kind      EntryKind
}

type Topic struct {
	Name          string
	Kind          Kind
	Aliases       []string
	Title         string
	Synopsis      string
	Detail        string
	Entries       []Entry
	Example       string
	SeeAlso       []string
	RuntimeModule string
	RuntimeGlobal string
	Requireable   bool
}

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

func All() []Topic { return registry }

func Topics(k Kind) []Topic {
	var out []Topic
	for _, t := range registry {
		if t.Kind == k || (k == KindModule && t.Requireable) {
			out = append(out, t)
		}
	}
	return out
}

func LookupTopic(name string) (*Topic, bool) {
	t, ok := index[name]
	return t, ok
}

func LookupEntry(qualified string) (*Topic, *Entry, bool) {
	ns, member, ok := splitQualified(qualified)
	if !ok {
		return nil, nil, false
	}
	return lookupIn(ns, member)
}

func Lookup(query string) (*Topic, *Entry, bool) {
	if t, ok := LookupTopic(query); ok {
		return t, nil, true
	}
	if t, e, ok := LookupEntry(query); ok {
		return t, e, true
	}
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

func splitQualified(s string) (ns, member string, ok bool) {
	i := strings.LastIndexAny(s, ".:")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

type Result struct {
	Topic *Topic
	Entry *Entry
}

func (r Result) Name() string {
	if r.Entry == nil {
		return r.Topic.Name
	}
	return r.Topic.Name + "." + r.Entry.Name
}

func (r Result) Summary() string {
	if r.Entry == nil {
		return r.Topic.Title
	}
	return r.Entry.Summary
}

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

func (t *Topic) Functions() []Entry { return t.entriesOfKind(EntryFunction, EntryMethod) }

func (t *Topic) Keywords() []Entry { return t.entriesOfKind(EntryKeyword) }

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
