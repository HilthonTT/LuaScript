package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	deferTok := p.curToken
	p.nextToken()

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

func (p *Parser) parseTryCatchStatement() ast.Statement {
	tryTok := p.curToken
	p.nextToken()

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
	p.nextToken()

	if p.curTokenIs(token.Ident) {
		stmt.CatchVar = &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal}
		p.nextToken()
	}

	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "try",
			"expected 'do' after 'catch', got "+describeToken(p.curToken),
			"syntax: `catch err do <handler> end` — the error binding is optional, the `do` is not")
		return nil
	}
	p.nextToken()

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
	p.nextToken()
	return stmt
}

func (p *Parser) parseThrowStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

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
