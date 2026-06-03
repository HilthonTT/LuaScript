// Package formatter is a pretty-printer for luascript source.
//
// The pipeline is:
//
//  1. Parse the input with compiler/parser → *ast.Program.
//  2. Scan the raw source for comments and blank lines (the parser drops
//     them) → []Trivia.
//  3. Walk the AST + trivia into a Doc IR (formatter/doc.go) describing
//     candidate layouts.
//  4. Render the Doc to a string with a width-aware best-fit algorithm
//     (formatter/render.go).
//
// Known v1 limitations (tracked as follow-ups; see the goal memory):
//   - Compound assignments (`x += 1`) round-trip as `x = x + 1` because the
//     parser desugars them before we see them.
//   - Inline comments that share a line with code (`foo() -- note`) are
//     emitted on the line above. Block-boundary comments and blank-line
//     separators are preserved correctly.
//   - String literal token text is normalized to Go-style `strconv.Quote`
//     for short strings; long-bracket strings (`[[...]]`) keep their form.
package formatter

import (
	"github.com/hilthontt/luascript/compiler/lexer"
	"github.com/hilthontt/luascript/compiler/parser"
)

// Format reformats `src` and returns the canonical form. A non-nil error
// is returned only when the parser rejects the input; in that case the
// caller should fall back to the original source rather than writing the
// partial output.
func Format(src string, opts Options) (string, error) {
	l := lexer.New(src)
	p := parser.New(l)
	prog, perr := p.ParseProgram()
	if perr != nil {
		return "", perr
	}
	triv := scanTrivia(src)
	e := &emitter{trivia: triv}
	// Skip any leading prelude trivia attached to lines before the first
	// statement so the formatted output doesn't start with stray blanks.
	if prog != nil && prog.Block != nil {
		var first int
		switch {
		case len(prog.Block.Statements) > 0:
			first = prog.Block.Statements[0].Line()
		case prog.Block.Return != nil:
			first = prog.Block.Return.Line()
		default:
			first = 1 << 30
		}
		// Keep leading comments (e.g. mode directive `--!strict`) but drop
		// any leading blank lines.
		preserved := e.emitLeadingPreserveComments(first)
		body := e.blockContent(prog.Block, opts)
		tail := e.flushRemainingTrivia()

		doc := concat(preserved, body, tail, hardLine())
		return renderDoc(doc, opts.width()), nil
	}
	// No block — just emit any trivia we found.
	doc := concat(e.flushRemainingTrivia(), hardLine())
	return renderDoc(doc, opts.width()), nil
}

// emitLeadingPreserveComments is a special variant for the file head: it
// keeps comment trivia (so `--!strict` survives) but discards every
// blank-line entry, because a formatted file shouldn't start with empty
// lines.
func (e *emitter) emitLeadingPreserveComments(upto int) Doc {
	var parts []Doc
	for e.ti < len(e.trivia) && e.trivia[e.ti].Line < upto {
		t := e.trivia[e.ti]
		e.ti++
		if t.Kind == BlankLine {
			continue
		}
		parts = append(parts, e.triviaDoc(t), hardLine())
	}
	if len(parts) == 0 {
		return nilDoc()
	}
	return concat(parts...)
}
