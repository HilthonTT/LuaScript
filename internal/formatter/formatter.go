package formatter

import (
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func Format(src string, opts Options) (string, error) {
	l := lexer.New(src)
	p := parser.New(l)
	prog, perr := p.ParseProgram()
	if perr != nil {
		return "", perr
	}
	triv := scanTrivia(src)
	e := &emitter{trivia: triv}
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
		preserved := e.emitLeadingPreserveComments(first)
		body := e.blockContent(prog.Block, opts)
		tail := e.flushRemainingTrivia()

		doc := concat(preserved, body, tail, hardLine())
		return renderDoc(doc, opts.width()), nil
	}
	doc := concat(e.flushRemainingTrivia(), hardLine())
	return renderDoc(doc, opts.width()), nil
}

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
