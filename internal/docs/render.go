package docs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hilthontt/luascript/internal/version"
)

type Options struct {
	Width int
	Color bool
}

const (
	bodyIndent  = "       "
	entryIndent = "              "
	minWidth    = 40
)

func (o Options) width() int {
	if o.Width < minWidth {
		if o.Width == 0 {
			return 80
		}
		return minWidth
	}
	return o.Width
}

func (o Options) bold(s string) string {
	if !o.Color || s == "" {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

func (o Options) dim(s string) string {
	if !o.Color || s == "" {
		return s
	}
	return "\033[2m" + s + "\033[0m"
}

func RenderTopic(t *Topic, o Options) string {
	var b strings.Builder
	b.WriteString(header(t, o))
	b.WriteString("\n\n")

	section(&b, o, "NAME")
	para(&b, o, bodyIndent, t.Name+" — "+t.Title)

	if t.Synopsis != "" {
		b.WriteString("\n")
		section(&b, o, "SYNOPSIS")
		verbatim(&b, bodyIndent, t.Synopsis)
	}

	if t.Detail != "" {
		b.WriteString("\n")
		section(&b, o, "DESCRIPTION")
		body(&b, o, bodyIndent, t.Detail)
	}

	fns, kws, consts := t.Functions(), t.Keywords(), t.Constants()
	if len(kws) > 0 {
		b.WriteString("\n")
		section(&b, o, "CONSTRUCTS")
		entries(&b, o, kws)
	}
	if len(fns) > 0 {
		b.WriteString("\n")
		if t.Kind == KindObject {
			section(&b, o, "METHODS")
		} else {
			section(&b, o, "FUNCTIONS")
		}
		entries(&b, o, fns)
	}
	if len(consts) > 0 {
		b.WriteString("\n")
		if t.Kind == KindObject {
			section(&b, o, "FIELDS")
		} else {
			section(&b, o, "CONSTANTS")
		}
		entries(&b, o, consts)
	}

	if t.Example != "" {
		b.WriteString("\n")
		section(&b, o, "EXAMPLES")
		verbatim(&b, bodyIndent, t.Example)
	}

	if len(t.SeeAlso) > 0 {
		b.WriteString("\n")
		section(&b, o, "SEE ALSO")
		para(&b, o, bodyIndent, strings.Join(t.SeeAlso, ", "))
	}

	b.WriteString("\n")
	b.WriteString(footer(t, o))
	b.WriteString("\n")
	return b.String()
}

func RenderEntry(t *Topic, e *Entry, o Options) string {
	var b strings.Builder
	b.WriteString(header(t, o))
	b.WriteString("\n\n")

	section(&b, o, "NAME")
	para(&b, o, bodyIndent, t.Name+"."+e.Name+" — "+firstSentence(e.Summary))

	b.WriteString("\n")
	section(&b, o, "SYNOPSIS")
	verbatim(&b, bodyIndent, e.Signature)

	b.WriteString("\n")
	section(&b, o, "DESCRIPTION")
	body(&b, o, bodyIndent, e.Summary)
	if e.Detail != "" {
		b.WriteString("\n")
		body(&b, o, bodyIndent, e.Detail)
	}

	b.WriteString("\n")
	section(&b, o, "SEE ALSO")
	see := append([]string{t.Name}, t.SeeAlso...)
	para(&b, o, bodyIndent, strings.Join(see, ", "))

	b.WriteString("\n")
	b.WriteString(footer(t, o))
	b.WriteString("\n")
	return b.String()
}

func RenderIndex(o Options) string {
	var b strings.Builder
	b.WriteString(o.bold("luascript documentation"))
	b.WriteString(" — ")
	b.WriteString(version.Version)
	b.WriteString("\n")
	b.WriteString("\n")

	groups := []struct {
		kind  Kind
		title string
	}{
		{KindCore, "BASE LIBRARY"},
		{KindLibrary, "AUTO-GLOBAL LIBRARIES"},
		{KindModule, "MODULES (require)"},
		{KindObject, "OBJECTS"},
	}
	for _, g := range groups {
		ts := Topics(g.kind)
		if len(ts) == 0 {
			continue
		}
		section(&b, o, g.title)
		width := 0
		for _, t := range ts {
			if len(t.Name) > width {
				width = len(t.Name)
			}
		}
		for _, t := range ts {
			line := fmt.Sprintf("%s%-*s  %s", bodyIndent, width, t.Name, t.Title)
			b.WriteString(truncate(line, o.width()))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(o.dim("Use \"luascript doc <topic>\" for a page, \"luascript doc <topic>.<name>\"\n"))
	b.WriteString(o.dim("for one entry, or \"luascript doc -k <text>\" to search."))
	b.WriteString("\n")
	return b.String()
}

func RenderSearch(results []Result, o Options) string {
	if len(results) == 0 {
		return ""
	}
	width := 0
	for _, r := range results {
		if n := len(r.Name()); n > width {
			width = n
		}
	}
	if width > 28 {
		width = 28
	}
	var b strings.Builder
	for _, r := range results {
		name := r.Name()
		pad := max(width-len(name), 0)
		line := o.bold(name) + strings.Repeat(" ", pad) + "  " + firstSentence(r.Summary())
		b.WriteString(truncateVisible(line, o.width()))
		b.WriteString("\n")
	}
	return b.String()
}

func RenderAll(o Options) string {
	var b strings.Builder
	for i, t := range registry {
		if i > 0 {
			b.WriteString("\n\f\n")
		}
		b.WriteString(RenderTopic(&t, o))
	}
	return b.String()
}

func TopicNames() []string {
	out := make([]string, 0, len(index))
	for k := range index {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func Suggest(query string) []string {
	q := strings.ToLower(query)
	if q == "" {
		return nil
	}
	candidates := suggestionCandidates()

	var prefix, sub []string
	for _, name := range candidates {
		l := strings.ToLower(name)
		switch {
		case strings.HasPrefix(l, q):
			prefix = append(prefix, name)
		case strings.Contains(l, q):
			sub = append(sub, name)
		}
	}
	out := append(prefix, sub...)

	if len(out) == 0 {
		budget := min(max(len(q)/4, 1), 3)
		for _, name := range candidates {
			if editDistanceWithin(q, strings.ToLower(name), budget) {
				out = append(out, name)
			}
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func suggestionCandidates() []string {
	out := TopicNames()
	for i := range registry {
		t := &registry[i]
		for j := range t.Entries {
			out = append(out, t.Name+"."+t.Entries[j].Name)
		}
	}
	sort.Strings(out)
	return out
}

func editDistanceWithin(a, b string, budget int) bool {
	if abs(len(a)-len(b)) > budget {
		return false
	}
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
			rowMin = min(rowMin, cur[j])
		}
		if rowMin > budget {
			return false
		}
		prev, cur = cur, prev
	}
	return prev[len(br)] <= budget
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func header(t *Topic, o Options) string {
	left := strings.ToUpper(t.Name) + "(3)"
	return banner(left, string(t.Kind), left, o)
}

func footer(t *Topic, o Options) string {
	right := strings.ToUpper(t.Name) + "(3)"
	return banner("luascript "+version.Version, "", right, o)
}

func banner(left, center, right string, o Options) string {
	w := o.width()
	if len(left)+len(center)+len(right)+2 > w {
		return o.bold(left)
	}
	lead := (w-len(center))/2 - len(left)
	if lead < 1 {
		lead = 1
	}
	trail := w - len(left) - lead - len(center) - len(right)
	if trail < 1 {
		trail = 1
	}
	return o.bold(left) + strings.Repeat(" ", lead) + o.bold(center) +
		strings.Repeat(" ", trail) + o.bold(right)
}

func section(b *strings.Builder, o Options, name string) {
	b.WriteString(o.bold(name))
	b.WriteString("\n")
}

func para(b *strings.Builder, o Options, indent, text string) {
	for _, line := range wrap(text, o.width()-len(indent)) {
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func body(b *strings.Builder, o Options, indent, text string) {
	blocks := strings.Split(strings.TrimRight(text, "\n"), "\n\n")
	for i, block := range blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		if isPreformatted(block) {
			verbatim(b, indent, dedent(block))
			continue
		}
		para(b, o, indent, strings.Join(strings.Fields(block), " "))
	}
}

func isPreformatted(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "  ") {
			return false
		}
	}
	return true
}

func dedent(block string) string {
	lines := strings.Split(block, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "  ")
	}
	return strings.Join(lines, "\n")
}

func verbatim(b *strings.Builder, indent, text string) {
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func entries(b *strings.Builder, o Options, es []Entry) {
	for i, e := range es {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(bodyIndent)
		b.WriteString(o.bold(e.Signature))
		b.WriteString("\n")
		para(b, o, entryIndent, e.Summary)
		if e.Detail != "" {
			b.WriteString("\n")
			body(b, o, entryIndent, e.Detail)
		}
	}
}

func wrap(text string, width int) []string {
	if width < 20 {
		width = 20
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var (
		lines []string
		cur   strings.Builder
		n     int
	)
	for _, w := range words {
		lw := len([]rune(w))
		if n > 0 && n+1+lw > width {
			lines = append(lines, cur.String())
			cur.Reset()
			n = 0
		}
		if n > 0 {
			cur.WriteByte(' ')
			n++
		}
		cur.WriteString(w)
		n += lw
	}
	if n > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func truncate(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width < 4 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}

func truncateVisible(line string, width int) string {
	visible := len([]rune(stripANSI(line)))
	if visible <= width {
		return line
	}
	over := visible - width + 1
	r := []rune(line)
	if over >= len(r) {
		return line
	}
	return string(r[:len(r)-over]) + "…"
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\033' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}
