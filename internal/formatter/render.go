package formatter

import "strings"

func renderDoc(d Doc, width int) string {
	var b strings.Builder
	type item struct {
		indent int
		flat   bool
		doc    Doc
	}
	stack := []item{{indent: 0, flat: false, doc: d}}
	col := 0

	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch v := top.doc.(type) {
		case docNil:
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
		case docNest:
			stack = append(stack, item{indent: top.indent + v.n, flat: top.flat, doc: v.inner})
		case docConcat:
			for i := len(v.parts) - 1; i >= 0; i-- {
				stack = append(stack, item{indent: top.indent, flat: top.flat, doc: v.parts[i]})
			}
		case docGroup:
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
			budget--
			if budget < 0 {
				return false
			}
		case docSoftLine:
		case docHardLine:
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
