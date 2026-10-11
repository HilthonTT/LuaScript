package parser

import (
	"github.com/hilthontt/luascript/v3/internal/compiler/ast"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/v3/internal/compiler/token"
)

const destructureSyntax = "local { a, b } = expr  or  local [ first, second ] = expr"

func (p *Parser) peekStartsExpression() bool {
	switch p.peekToken.Type {
	case token.Ident, token.Int, token.Float, token.String, token.InterpString,
		token.True, token.False, token.Nil, token.Vararg,
		token.LParen, token.LBrace, token.Function, token.If,
		token.Minus, token.Not, token.Hash, token.Tilde:
		return true
	}
	return false
}

func (p *Parser) parseLocalDestructure(localTok token.Token) ast.Statement {
	isArray := p.curTokenIs(token.LBracket)
	closer := token.RBrace
	if isArray {
		closer = token.RBracket
	}
	p.nextToken()

	stmt := &ast.LocalDestructureStatement{BaseNode: baseAt(localTok), IsArray: isArray}
	seen := map[string]bool{}
	position := 0

	for !p.curTokenIs(closer) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(localTok, errors.EndOfFileError, "local",
				"missing closing bracket in destructuring pattern",
				"syntax: "+destructureSyntax)
			return nil
		}

		bind, ok := p.parseDestructureBind(isArray, &position)
		if !ok {
			return nil
		}
		if seen[bind.Bind] {
			p.errorAt(p.curToken, errors.SyntaxError, "local",
				"duplicate binding '"+bind.Bind+"' in destructuring pattern",
				"each name in the pattern must be unique")
			return nil
		}
		seen[bind.Bind] = true
		if bind.Rest {
			if !p.curTokenIs(closer) {
				p.errorAt(p.curToken, errors.SyntaxError, "local",
					"'...' must be the last entry of a destructuring pattern",
					"move the rest binding to the end: `{ a, ...others }`")
				return nil
			}
		}
		stmt.Binds = append(stmt.Binds, bind)

		if p.curTokenIs(token.Comma) || p.curTokenIs(token.Semicolon) {
			p.nextToken()
			continue
		}
		break
	}

	if !p.expectCur(closer) {
		return nil
	}
	p.nextToken()

	if len(stmt.Binds) == 0 {
		p.errorAt(localTok, errors.SyntaxError, "local",
			"destructuring pattern binds nothing",
			"syntax: "+destructureSyntax)
		return nil
	}

	if !p.expectCur(token.Assign) {
		return nil
	}
	p.nextToken()

	stmt.Value = p.parseExpression()
	if stmt.Value == nil {
		return nil
	}
	return stmt
}

func (p *Parser) parseDestructureBind(isArray bool, position *int) (ast.DestructureBind, bool) {
	if p.curTokenIs(token.Vararg) {
		p.nextToken()
		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.SyntaxError, "local",
				"expected a name after '...', got "+describeToken(p.curToken),
				"a rest binding looks like `{ a, ...others }`")
			return ast.DestructureBind{}, false
		}
		name := p.curToken.Literal
		p.nextToken()
		return ast.DestructureBind{Bind: name, Rest: true, Index: *position + 1}, true
	}

	if !p.curTokenIs(token.Ident) {
		p.errorAt(p.curToken, errors.SyntaxError, "local",
			"expected a name in destructuring pattern, got "+describeToken(p.curToken),
			"syntax: "+destructureSyntax)
		return ast.DestructureBind{}, false
	}

	name := p.curToken.Literal
	p.nextToken()

	bind := ast.DestructureBind{Bind: name}
	if isArray {
		*position++
		bind.Index = *position
	} else {
		bind.Key = name
		if p.curTokenIs(token.Assign) {
			p.nextToken()
			if !p.curTokenIs(token.Ident) {
				p.errorAt(p.curToken, errors.SyntaxError, "local",
					"expected a name after '=' in destructuring pattern, got "+describeToken(p.curToken),
					"rename a field with `{ field = localName }`")
				return ast.DestructureBind{}, false
			}
			bind.Bind = p.curToken.Literal
			p.nextToken()
		}
	}

	if p.curTokenIs(token.Colon) {
		p.nextToken()
		bind.Type = p.parseType()
		if bind.Type == nil {
			return ast.DestructureBind{}, false
		}
	}

	if p.curTokenIs(token.Or) {
		p.nextToken()
		bind.Default = p.parseExpression()
		if bind.Default == nil {
			return ast.DestructureBind{}, false
		}
	}

	return bind, true
}
