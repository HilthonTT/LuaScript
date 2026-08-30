package parser

// Parsing for defer, try/catch, and throw.

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

// parseDeferStatement reads `defer <call>`. Like Go's defer it accepts only a
// function or method call. The cursor is on `defer` at entry; whatever follows
// the call (including a trailing `;`) is left for the block loop to consume.
func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	deferTok := p.curToken
	p.nextToken() // consume 'defer'

	call := p.parseExpression()
	if call == nil {
		return nil
	}
	switch call.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression:
	default:
		p.errorAt(deferTok, errors.SyntaxError, "defer",
			"defer expects a function or method call",
			"syntax: defer cleanup(args)  or  defer obj:method(args)")
		return nil
	}
	return &ast.DeferStatement{BaseNode: baseAt(deferTok), Call: call}
}

// parseTryCatchStatement reads
//
//	try <body> catch [Name] do <handler> end
//
// `catch` is a block terminator (see endOfBlock), so the try body needs no
// `do`/`end` of its own. The error binding is optional, but `do` is required
// either way: a handler body may itself begin with an identifier, which would
// otherwise be indistinguishable from the binding name.
func (p *Parser) parseTryCatchStatement() ast.Statement {
	tryTok := p.curToken
	p.nextToken() // consume 'try'

	stmt := &ast.TryCatchStatement{BaseNode: baseAt(tryTok)}
	stmt.Try = p.parseBlock()
	if p.error != nil {
		return nil
	}

	if !p.curTokenIs(token.Catch) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "try",
			fmt.Sprintf("missing 'catch' to close 'try' started on line %d, got %s",
				tryTok.Line, describeToken(p.curToken)),
			"syntax: `try <body> catch err do <handler> end` — every `try` needs exactly one `catch`")
		return nil
	}
	p.nextToken() // consume 'catch'

	if p.curTokenIs(token.Ident) {
		stmt.CatchVar = &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal}
		p.nextToken() // consume the binding name
	}

	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "try",
			"expected 'do' after 'catch', got "+describeToken(p.curToken),
			"syntax: `catch err do <handler> end` — the error binding is optional, the `do` is not")
		return nil
	}
	p.nextToken() // consume 'do'

	stmt.Catch = p.parseBlock()
	if p.error != nil {
		return nil
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "try",
			fmt.Sprintf("missing 'end' to close 'try' started on line %d, got %s",
				tryTok.Line, describeToken(p.curToken)),
			"syntax: `try <body> catch err do <handler> end`")
		return nil
	}
	p.nextToken() // consume 'end'
	return stmt
}

// parseThrowStatement reads `throw <expr>`. The thrown value is arbitrary —
// any Lua value, not just a string — matching `error(v)`, which is what this
// lowers to.
func (p *Parser) parseThrowStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'throw'

	val := p.parseExpression()
	if p.error != nil {
		return nil
	}
	if val == nil {
		p.errorAt(tok, errors.UnexpectedTokenError, "throw",
			"expected a value to throw after 'throw', got "+describeToken(p.curToken),
			"syntax: `throw <expr>` — e.g. `throw \"boom\"` or `throw { code = 42 }`")
		return nil
	}
	return &ast.ThrowStatement{BaseNode: baseAt(tok), Value: val}
}
