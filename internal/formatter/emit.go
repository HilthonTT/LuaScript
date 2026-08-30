package formatter

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// emitter walks an ast.Program and produces a Doc. It also interleaves
// preserved trivia (comments, blank lines) by source line, since the parser
// drops them.
//
// Trivia interleaving works at block boundaries: before emitting a
// statement we flush every piece of trivia whose start line precedes the
// statement's start line. Inline trivia on the same line as a statement is
// out of scope for v1 — flagged in the package doc.
type emitter struct {
	trivia []Trivia
	ti     int // cursor into trivia
}

// Options controls formatter behavior. Empty zero value is "default style":
// 80-col target, 2-space indent. Kept tiny on purpose — once we ship more
// surface area we can grow it.
type Options struct {
	Width  int // target line width; 0 → 80
	Indent int // spaces per indent level; 0 → 2
}

func (o Options) width() int {
	if o.Width <= 0 {
		return 80
	}
	return o.Width
}

func (o Options) indent() int {
	if o.Indent <= 0 {
		return 2
	}
	return o.Indent
}

// flushRemainingTrivia drains any trivia past the last AST node. We drop
// trailing blank lines (the trailing newline is added separately) but keep
// any trailing comments.
func (e *emitter) flushRemainingTrivia() Doc {
	var parts []Doc
	first := true
	for e.ti < len(e.trivia) {
		t := e.trivia[e.ti]
		e.ti++
		if t.Kind == BlankLine {
			continue
		}
		if !first {
			parts = append(parts, hardLine())
		}
		parts = append(parts, e.triviaDoc(t))
		first = false
	}
	if len(parts) == 0 {
		return nilDoc()
	}
	return concat(append([]Doc{hardLine()}, parts...)...)
}

// triviaDoc renders one Trivia piece. BlankLine returns nilDoc — callers
// emit the actual hardLine separator.
func (e *emitter) triviaDoc(t Trivia) Doc {
	switch t.Kind {
	case LineComment:
		return text(normalizeLineComment(t.Text))
	case LongComment:
		return text(t.Text)
	default:
		return nilDoc()
	}
}

// block emits a nested block (function body, if-arm, loop body). The body
// is preceded by a hardLine and indented by one level so the caller can
// write `text("do"), e.block(body, opts), hardLine(), text("end")` and get
// the conventional layout.
func (e *emitter) block(b *ast.Block, opts Options) Doc {
	content := e.blockContent(b, opts)
	if _, ok := content.(docNil); ok {
		return nilDoc()
	}
	return nest(opts.indent(), concat(hardLine(), content))
}

// blockContent emits a block's lines without any surrounding indent or
// leading newline. Used for the chunk-level block (the outermost program)
// where indenting would be wrong, and as a helper from block().
//
// Each statement and each preserved comment becomes one "logical line".
// A run of one-or-more source blank lines collapses to a single empty
// logical line. The lines are joined with hardLine separators — a blank
// logical line therefore renders as a true blank line in the output.
func (e *emitter) blockContent(b *ast.Block, opts Options) Doc {
	if b == nil || b.IsEmpty() {
		return nilDoc()
	}
	var lines []Doc

	appendTriviaUntil := func(upto int) {
		prevBlank := false
		for e.ti < len(e.trivia) && e.trivia[e.ti].Line < upto {
			t := e.trivia[e.ti]
			e.ti++
			if t.Kind == BlankLine {
				// Drop blank lines at the very start of a block — those
				// were just visual padding under the opening keyword and
				// the formatter applies its own spacing.
				if prevBlank || len(lines) == 0 {
					continue
				}
				lines = append(lines, nilDoc())
				prevBlank = true
				continue
			}
			lines = append(lines, e.triviaDoc(t))
			prevBlank = false
		}
	}

	for i, stmt := range b.Statements {
		appendTriviaUntil(stmt.Line())
		doc := e.statement(stmt, opts)
		// Lua ignores line breaks, so a statement that begins with `(` fuses
		// onto whatever precedes it: `f(x)` followed by `("s"):upper()` parses
		// as a single call `f(x)("s"):upper()`. The source must have carried a
		// separating `;` to parse at all — re-emit one, or the formatted file
		// means something different from the file we were given.
		if i+1 < len(b.Statements) && statementStartsWithParen(b.Statements[i+1]) {
			doc = concat(doc, text(";"))
		}
		lines = append(lines, doc)
	}
	if b.Return != nil {
		appendTriviaUntil(b.Return.Line())
		lines = append(lines, e.returnStmt(b.Return, opts))
	}

	parts := make([]Doc, 0, len(lines)*2)
	for i, ln := range lines {
		if i > 0 {
			parts = append(parts, hardLine())
		}
		parts = append(parts, ln)
	}
	return concat(parts...)
}

// statementStartsWithParen reports whether a statement's first token is `(`.
// Only an expression statement or an assignment can start that way; every
// other statement begins with a keyword or a name.
func statementStartsWithParen(s ast.Statement) bool {
	switch n := s.(type) {
	case *ast.ExpressionStatement:
		return leadsWithParen(n.Expression)
	case *ast.AssignStatement:
		if len(n.Targets) == 0 {
			return false
		}
		return leadsWithParen(n.Targets[0])
	}
	return false
}

// leadsWithParen walks to the leftmost primary of an expression and reports
// whether it is a parenthesised one. `("s"):upper()` is a method call whose
// object is a ParenExpression, so the check has to descend rather than test
// the top node.
func leadsWithParen(e ast.Expression) bool {
	switch n := e.(type) {
	case *ast.ParenExpression:
		return true
	case *ast.CallExpression:
		return leadsWithParen(n.Func)
	case *ast.MethodCallExpression:
		return leadsWithParen(n.Object)
	case *ast.IndexExpression:
		return leadsWithParen(n.Object)
	case *ast.BinaryExpression:
		return leadsWithParen(n.Left)
	}
	return false
}
