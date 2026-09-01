package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func (p *Parser) parseEnumStatement() *ast.EnumStatement {
	enumTok := p.curToken
	stmt := &ast.EnumStatement{BaseNode: baseAt(enumTok)}

	if !p.expectPeek(token.Ident) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "enum",
			"expected an identifier after 'enum', got "+describeToken(p.curToken),
			"syntax: enum Name VARIANT, VARIANT, ... end")
		return nil
	}

	stmt.Name = &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal}

	p.nextToken()

	seen := map[string]bool{}
	for !p.curTokenIs(token.End) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(enumTok, errors.EndOfFileError, "enum",
				fmt.Sprintf("missing 'end' to close 'enum %s' started on line %d", stmt.Name.Name, enumTok.Line),
				"close the variant list with `end`: `enum Color RED, GREEN, BLUE end`")
			return nil
		}

		if p.curTokenIs(token.Comma) {
			p.nextToken()
			continue
		}

		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "enum",
				"expected a variant identifier or 'end', got "+describeToken(p.curToken),
				"variants are bare identifiers separated by commas: `RED, GREEN, BLUE`")
			return nil
		}

		name := p.curToken.Literal
		if seen[name] {
			p.errorAt(p.curToken, errors.SyntaxError, "enum",
				fmt.Sprintf("duplicate variant '%s' in enum '%s'", name, stmt.Name.Name),
				"each variant must be unique within its enum")
			return nil
		}
		seen[name] = true
		p.nextToken()

		variant := &ast.EnumVariantDef{Name: name}
		if p.curTokenIs(token.LParen) {
			payload, ok := p.parseEnumVariantPayload(name)
			if !ok {
				return nil
			}
			variant.Payload = payload
			if p.enumVariants == nil {
				p.enumVariants = make(map[string]bool)
			}
			p.enumVariants[name] = true
		}
		stmt.Variants = append(stmt.Variants, variant)
	}

	if len(stmt.Variants) == 0 {
		p.errorAt(enumTok, errors.SyntaxError, "enum",
			fmt.Sprintf("enum '%s' has no variants", stmt.Name.Name),
			"add at least one variant: `enum Color RED end`")
		return nil
	}

	p.nextToken()
	return stmt
}

func (p *Parser) parseEnumVariantPayload(variant string) ([]ast.TypeNode, bool) {
	p.nextToken()
	if p.curTokenIs(token.RParen) {
		p.errorAt(p.curToken, errors.SyntaxError, "enum",
			fmt.Sprintf("variant '%s' has an empty payload '()'", variant),
			"omit the parentheses for a payload-less variant: `"+variant+"`")
		return nil, false
	}
	var payload []ast.TypeNode
	for {
		ty := p.parseType()
		if ty == nil {
			return nil, false
		}
		payload = append(payload, ty)
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken()
	}
	if !p.curTokenIs(token.RParen) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "enum",
			"expected ')' to close the payload of variant '"+variant+"', got "+describeToken(p.curToken),
			"payloads look like `Circle(number)` or `Rect(number, number)`")
		return nil, false
	}
	p.nextToken()
	return payload, true
}
