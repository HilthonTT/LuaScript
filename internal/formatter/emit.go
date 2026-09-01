package formatter

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type emitter struct {
	trivia []Trivia
	ti     int
}

type Options struct {
	Width  int
	Indent int
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

func (e *emitter) block(b *ast.Block, opts Options) Doc {
	content := e.blockContent(b, opts)
	if _, ok := content.(docNil); ok {
		return nilDoc()
	}
	return nest(opts.indent(), concat(hardLine(), content))
}

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
