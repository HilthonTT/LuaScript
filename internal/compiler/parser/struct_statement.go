package parser

// `struct` is a nominal product type: a fixed set of named, typed fields
// plus a constructor. Like `type`, `struct` is a *soft* keyword recognised
// only in the `struct <Ident>` position (see parseStatement) so it stays a
// legal identifier everywhere else.
//
// Grammar:
//
//	struct Name [ '<' T { ',' T } '>' ] '{'
//	    field ':' Type [ ',' | ';' ]
//	    ...
//	'}'
//
// The declaration lowers to a constructor bound to `Name` (see the bytecode
// generator's compileStructStatement) and registers `Name` as both a type
// alias for the structural table and a constructor function in the checker.

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

const structSyntax = "struct Name { field: Type, ... }"

// parseStructStatement consumes a `struct Name { ... }` declaration. The
// cursor is on the `struct` soft keyword at entry.
func (p *Parser) parseStructStatement() ast.Statement {
	structTok := p.curToken
	p.nextToken() // consume 'struct'

	if !p.curTokenIs(token.Ident) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "struct",
			"expected a name after 'struct', got "+describeToken(p.curToken),
			"syntax: "+structSyntax)
		return nil
	}
	stmt := &ast.StructStatement{
		BaseNode: baseAt(structTok),
		Name:     &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal},
	}
	// Register the name so match patterns can tell struct destructures
	// apart from ordinary call-shaped value patterns.
	if p.structNames == nil {
		p.structNames = make(map[string]bool)
	}
	p.structNames[stmt.Name.Name] = true
	p.nextToken() // consume name

	// Optional generic parameter list `<T, U>`.
	if p.curTokenIs(token.LT) {
		stmt.TypeParams = p.parseTypeParams()
		if p.error != nil {
			return nil
		}
	}

	if !p.curTokenIs(token.LBrace) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "struct",
			"expected '{' to open the field list, got "+describeToken(p.curToken),
			"syntax: "+structSyntax)
		return nil
	}
	p.nextToken() // consume '{'

	seen := map[string]bool{}
	for !p.curTokenIs(token.RBrace) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(structTok, errors.EndOfFileError, "struct",
				"missing '}' to close struct '"+stmt.Name.Name+"'",
				"close the field list with '}'")
			return nil
		}

		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "struct",
				"expected a field name, got "+describeToken(p.curToken),
				"each field is `name: Type`, separated by ',' or ';'")
			return nil
		}
		name := p.curToken.Literal
		if seen[name] {
			p.errorAt(p.curToken, errors.SyntaxError, "struct",
				"duplicate field '"+name+"' in struct '"+stmt.Name.Name+"'",
				"each field name must be unique within its struct")
			return nil
		}
		seen[name] = true
		p.nextToken() // consume field name

		if !p.curTokenIs(token.Colon) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "struct",
				"expected ':' after field name '"+name+"', got "+describeToken(p.curToken),
				"struct fields are always typed: `"+name+": number`")
			return nil
		}
		p.nextToken() // consume ':'

		fieldType := p.parseType()
		if fieldType == nil {
			return nil
		}
		stmt.Fields = append(stmt.Fields, ast.StructField{Name: name, Type: fieldType})

		// Comma / semicolon separators, trailing separator optional.
		if p.curTokenIs(token.Comma) || p.curTokenIs(token.Semicolon) {
			p.nextToken()
		}
	}

	if len(stmt.Fields) == 0 {
		p.errorAt(structTok, errors.SyntaxError, "struct",
			"struct '"+stmt.Name.Name+"' has no fields",
			"add at least one field: `struct "+stmt.Name.Name+" { value: number }`")
		return nil
	}

	p.nextToken() // consume '}'
	return stmt
}
