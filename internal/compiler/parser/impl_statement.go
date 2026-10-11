package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/v2/internal/compiler/ast"
	"github.com/hilthontt/luascript/v2/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/v2/internal/compiler/token"
)

const implSyntax = "impl Name\n    function method(self) ... end\nend"

func (p *Parser) parseImplStatement() ast.Statement {
	implTok := p.curToken
	p.nextToken()

	if !p.curTokenIs(token.Ident) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "impl",
			"expected a name after 'impl', got "+describeToken(p.curToken),
			"syntax: "+implSyntax)
		return nil
	}
	stmt := &ast.ImplStatement{
		BaseNode: baseAt(implTok),
		Target:   &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal},
	}
	p.nextToken()

	seen := map[string]bool{}
	for !p.curTokenIs(token.End) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(implTok, errors.EndOfFileError, "impl",
				fmt.Sprintf("missing 'end' to close 'impl %s' started on line %d",
					stmt.Target.Name, implTok.Line),
				"syntax: "+implSyntax)
			return nil
		}
		if p.curTokenIs(token.Semicolon) {
			p.nextToken()
			continue
		}
		if !p.curTokenIs(token.Function) {
			p.errorAt(p.curToken, errors.SyntaxError, "impl",
				"an impl block holds only function declarations, got "+describeToken(p.curToken),
				"syntax: "+implSyntax)
			return nil
		}

		fnTok := p.curToken
		p.nextToken()
		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "impl",
				"expected a member name, got "+describeToken(p.curToken),
				"syntax: "+implSyntax)
			return nil
		}
		name := p.curToken.Literal
		if seen[name] {
			p.errorAt(p.curToken, errors.SyntaxError, "impl",
				"duplicate member '"+name+"' in impl block for '"+stmt.Target.Name+"'",
				"each member name must be unique within its impl block")
			return nil
		}
		seen[name] = true
		p.nextToken()

		body := p.parseFunctionBody(fnTok)
		if body == nil {
			return nil
		}
		stmt.Members = append(stmt.Members, ast.ImplMember{Name: name, Func: body})
	}
	p.nextToken()

	if len(stmt.Members) == 0 {
		p.errorAt(implTok, errors.SyntaxError, "impl",
			"impl block for '"+stmt.Target.Name+"' is empty",
			"syntax: "+implSyntax)
		return nil
	}
	return stmt
}
