package parser

import (
	"github.com/hilthontt/luascript/v2/internal/compiler/ast"
	"github.com/hilthontt/luascript/v2/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/v2/internal/compiler/token"
)

const interfaceSyntax = "interface Name { field: Type, method: (self) -> Type }"

func (p *Parser) parseInterfaceStatement() ast.Statement {
	ifaceTok := p.curToken
	p.nextToken()

	if !p.curTokenIs(token.Ident) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "interface",
			"expected a name after 'interface', got "+describeToken(p.curToken),
			"syntax: "+interfaceSyntax)
		return nil
	}
	name := p.curToken.Literal
	p.nextToken()

	var typeParams []ast.TypeParam
	if p.curTokenIs(token.LT) {
		typeParams = p.parseTypeParams()
		if p.error != nil {
			return nil
		}
	}

	if !p.curTokenIs(token.LBrace) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "interface",
			"expected '{' to open the interface body, got "+describeToken(p.curToken),
			"syntax: "+interfaceSyntax)
		return nil
	}
	target := p.parseTableType()
	if target == nil {
		return nil
	}
	return &ast.TypeAliasStatement{
		BaseNode:    baseAt(ifaceTok),
		Name:        name,
		TypeParams:  typeParams,
		Target:      target,
		IsInterface: true,
	}
}
