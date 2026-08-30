package parser

// Parsing for enum declarations and tagged-variant payloads.

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

//	end
//
// and returns an *ast.EnumStatement. v1 is integer auto-increment, so
// the AST only carries variant names; values are assigned at lowering
// time (RED → 1, GREEN → 2, …). The trailing comma after the last
// variant is optional. Duplicate variant names are reported at parse
// time — they would either silently clobber or produce conflicting
// table fields downstream, and the error here is much clearer than
// either downstream symptom.
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

	// Move past the name. From here we accept a sequence of
	// `Ident [,]` until we see `end`. A leading newline / comma between
	// the name and the first variant is fine; the loop tolerates
	// stray commas implicitly (a comma alone advances and re-enters).
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
			// A comma between variants is fine; a leading or duplicate
			// comma is harmless and skipping it keeps the error
			// reporting focused on actual structural problems.
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
		p.nextToken() // consume variant name

		variant := &ast.EnumVariantDef{Name: name}
		// Optional payload: `Circle(number)`, `Rect(number, number)`. A
		// variant with a payload makes the whole enum a tagged sum type.
		if p.curTokenIs(token.LParen) {
			payload, ok := p.parseEnumVariantPayload(name)
			if !ok {
				return nil
			}
			variant.Payload = payload
			// Register the variant so match patterns can tell positional
			// destructures (`Circle(r)`) apart from call-shaped value
			// patterns (`f(x)`).
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

	p.nextToken() // consume 'end'
	return stmt
}

// parseEnumVariantPayload reads a tagged variant's `(Type {, Type})` payload.
// The cursor is on `(` at entry. An empty `()` is rejected — a variant with
// no payload should just be written bare (`Unit`, not `Unit()`).
func (p *Parser) parseEnumVariantPayload(variant string) ([]ast.TypeNode, bool) {
	p.nextToken() // consume '('
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
		p.nextToken() // consume ','
	}
	if !p.curTokenIs(token.RParen) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "enum",
			"expected ')' to close the payload of variant '"+variant+"', got "+describeToken(p.curToken),
			"payloads look like `Circle(number)` or `Rect(number, number)`")
		return nil, false
	}
	p.nextToken() // consume ')'
	return payload, true
}
