package formatter

import "strings"

// renderDoc lays out d into a string within the given target width.
//
// The algorithm is Lindig's "Strictly Pretty": a work-list of pending pieces,
// each tagged with an indent level and a current mode (flat or break). For a
// Group we measure whether the group fits in the remaining width with all
// inner Lines treated as spaces; if so, we commit the group to flat mode,
// otherwise to break mode. The decision is local — nested groups make their
// own choice when their turn comes — which is why long-tail breaks compose
// correctly.
//
// The implementation runs in O(width) per group thanks to the early-exit in
// fits(): we stop measuring as soon as we've consumed more width than the
// budget. That keeps formatting linear in input size for typical sources.
func renderDoc(d Doc, width int) string {
	var b strings.Builder
	type item struct {
		indent int
		flat   bool // true == flat mode (Lines render as spaces)
		doc    Doc
	}
	stack := []item{{indent: 0, flat: false, doc: d}}
	col := 0

	for len(stack) > 0 {
		// Pop the top of stack.
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch v := top.doc.(type) {
		case docNil:
			// nothing
		case docText:
			b.WriteString(v.s)
			col += len(v.s)
		case docHardLine:
			b.WriteByte('\n')
			writeIndent(&b, top.indent)
			col = top.indent
		case docLine:
			if top.flat {
				b.WriteByte(' ')
				col++
			} else {
				b.WriteByte('\n')
				writeIndent(&b, top.indent)
				col = top.indent
			}
		case docSoftLine:
			if !top.flat {
				b.WriteByte('\n')
				writeIndent(&b, top.indent)
				col = top.indent
			}
			// flat: emit nothing
		case docNest:
			stack = append(stack, item{indent: top.indent + v.n, flat: top.flat, doc: v.inner})
		case docConcat:
			// Push in reverse so we process left-to-right.
			for i := len(v.parts) - 1; i >= 0; i-- {
				stack = append(stack, item{indent: top.indent, flat: top.flat, doc: v.parts[i]})
			}
		case docGroup:
			// Try flat. If it fits, commit; else fall back to break.
			budget := width - col
			if fits(v.inner, top.indent, budget) {
				stack = append(stack, item{indent: top.indent, flat: true, doc: v.inner})
			} else {
				stack = append(stack, item{indent: top.indent, flat: false, doc: v.inner})
			}
		}
	}
	return b.String()
}

// fits reports whether `d` rendered in flat mode at the given indent will
// consume no more than `budget` columns before the first line break.
//
// HardLines force a break immediately (returns true if we got there under
// budget — the rest is on a new line and doesn't count against this fit).
// A Group nested inside is *also* measured flat: this is the simple-strict
// variant; it overshoots only on pathologically nested groups, which is
// fine for Lua-shaped code.
func fits(d Doc, indent, budget int) bool {
	if budget < 0 {
		return false
	}
	type item struct {
		indent int
		doc    Doc
	}
	stack := []item{{indent: indent, doc: d}}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch v := top.doc.(type) {
		case docNil:
		case docText:
			budget -= len(v.s)
			if budget < 0 {
				return false
			}
		case docLine:
			// Flat: counts as a single space.
			budget--
			if budget < 0 {
				return false
			}
		case docSoftLine:
			// Flat: empty.
		case docHardLine:
			// A hard break short-circuits "fits": the prefix fit, the rest
			// is irrelevant to *this* group's flat decision.
			return true
		case docNest:
			stack = append(stack, item{indent: top.indent + v.n, doc: v.inner})
		case docConcat:
			for i := len(v.parts) - 1; i >= 0; i-- {
				stack = append(stack, item{indent: top.indent, doc: v.parts[i]})
			}
		case docGroup:
			stack = append(stack, item{indent: top.indent, doc: v.inner})
		}
	}
	return true
}

func writeIndent(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}
