package parser

import (
	"github.com/hilthontt/luascript/compiler/ast"
	"github.com/hilthontt/luascript/compiler/parser/errors"
	"github.com/hilthontt/luascript/compiler/token"
)

// parseType is the entry point for a type expression. It handles `|` unions
// left-to-right around parseTypeAtom (which itself handles the `?` postfix).
//
//	T   :=  Atom { '|' Atom }
//	Atom := Primitive | Name | '(' ... ')' [ '->' Returns ] | TableType
//	      | Atom '?'
//
// The function returns nil and records a parser error on failure, following
// the same convention as the rest of the parser.
func (p *Parser) parseType() ast.TypeNode {
	first := p.parseTypeAtom()
	if first == nil {
		return nil
	}
	if !p.curTokenIs(token.Pipe) {
		return first
	}
	startTok := p.curToken
	members := []ast.TypeNode{first}
	for p.curTokenIs(token.Pipe) {
		p.nextToken() // consume '|'
		next := p.parseTypeAtom()
		if next == nil {
			return nil
		}
		members = append(members, next)
	}
	return &ast.TypeUnion{BaseNode: baseAt(startTok), Members: members}
}

// parseTypeAtom parses one atomic type, then wraps it in TypeOptional for
// each trailing `?`.
func (p *Parser) parseTypeAtom() ast.TypeNode {
	var t ast.TypeNode

	switch {
	case p.curTokenIs(token.LParen):
		t = p.parseParenOrFunctionType()
	case p.curTokenIs(token.LBrace):
		t = p.parseTableType()
	case p.curTokenIs(token.Nil):
		// `nil` as a type is a literal type (used in unions like `T | nil`).
		// Modeled as a primitive named "nil".
		tok := p.curToken
		p.nextToken()
		t = &ast.TypePrimitive{BaseNode: baseAt(tok), Name: "nil"}
	case p.curTokenIs(token.Ident):
		// Identifier-led atom: distinguish built-in primitives from user
		// alias references purely by name. The set of primitives is closed.
		tok := p.curToken
		name := p.curToken.Literal
		p.nextToken()
		if isPrimitiveTypeName(name) {
			t = &ast.TypePrimitive{BaseNode: baseAt(tok), Name: name}
		} else {
			var args []ast.TypeNode
			if p.curTokenIs(token.LT) {
				args = p.parseTypeArgList()
				if p.error != nil {
					return nil
				}
			}
			t = &ast.TypeName{BaseNode: baseAt(tok), Name: name, TypeArgs: args}
		}
	default:
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected a type, got "+describeToken(p.curToken),
			"valid types: a name (`number`, `MyAlias`), a function type `(A) -> B`, a table `{ x: T }`, or a union `A | B`")
		return nil
	}

	if t == nil {
		return nil
	}

	// Postfix `?` — possibly stacked, though `T??` adds nothing semantically.
	for p.curTokenIs(token.Question) {
		tok := p.curToken
		p.nextToken()
		t = &ast.TypeOptional{BaseNode: baseAt(tok), Inner: t}
	}
	return t
}

// parseParenOrFunctionType handles a `(` opener that may be either a
// parenthesized type or a function type's parameter list. The disambiguator
// is the token immediately after the matching `)`: an `->` means function.
//
//	'(' [ Param { ',' Param } [ ',' Vararg ] | Vararg ] ')' [ '->' Returns ]
//	Param  := [ Name ':' ] T
//	Vararg := '...' [ ':' T ]
//	Returns := T | '(' [ T { ',' T } ] ')'
func (p *Parser) parseParenOrFunctionType() ast.TypeNode {
	openTok := p.curToken
	p.nextToken() // consume '('

	var paramNames []string
	var paramTypes []ast.TypeNode
	isVararg := false
	var varargType ast.TypeNode

	// Empty list `()` — only meaningful as a function type's empty params
	// or a no-return marker after `->`. The arrow check below handles both.
	if !p.curTokenIs(token.RParen) {
		for {
			if p.curTokenIs(token.Vararg) {
				p.nextToken()
				isVararg = true
				if p.curTokenIs(token.Colon) {
					p.nextToken()
					varargType = p.parseType()
					if varargType == nil {
						return nil
					}
				}
				break // `...` must be last
			}

			name := ""
			// `Name : T` form — named function-type parameter (Luau allows
			// names purely as documentation; the checker doesn't bind them).
			if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon) {
				name = p.curToken.Literal
				p.nextToken() // consume name
				p.nextToken() // consume ':'
			}
			ty := p.parseType()
			if ty == nil {
				return nil
			}
			paramNames = append(paramNames, name)
			paramTypes = append(paramTypes, ty)

			if !p.curTokenIs(token.Comma) {
				break
			}
			p.nextToken() // consume ','
		}
	}

	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken() // consume ')'

	// Function type: `(...) -> Returns`.
	if p.curTokenIs(token.Arrow) {
		p.nextToken() // consume '->'
		returns := p.parseReturnTypeList()
		if p.error != nil {
			return nil
		}
		return &ast.TypeFunction{
			BaseNode:   baseAt(openTok),
			ParamNames: paramNames,
			Params:     paramTypes,
			Returns:    returns,
			IsVararg:   isVararg,
			VarargType: varargType,
		}
	}

	// Plain parenthesized type — must be exactly one positional unnamed
	// element, no vararg.
	if isVararg || len(paramTypes) != 1 || (len(paramNames) > 0 && paramNames[0] != "") {
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected '->' after function-type parameter list",
			"function types are written `(<params>) -> <return>`, e.g. `(number, string) -> boolean`")
		return nil
	}
	return paramTypes[0]
}

// parseReturnTypeList reads the right-hand side of `->`. A single bare type
// or a parenthesized list of types — the latter is the multi-return form.
func (p *Parser) parseReturnTypeList() []ast.TypeNode {
	if !p.curTokenIs(token.LParen) {
		// Single return type.
		t := p.parseType()
		if t == nil {
			return nil
		}
		return []ast.TypeNode{t}
	}
	p.nextToken() // consume '('
	if p.curTokenIs(token.RParen) {
		p.nextToken() // consume ')'
		return nil    // `() -> ()` — no returns
	}
	var rets []ast.TypeNode
	for {
		t := p.parseType()
		if t == nil {
			return nil
		}
		rets = append(rets, t)
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken() // consume ','
	}
	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken() // consume ')'
	return rets
}

// parseTableType reads `{ ... }`. Three accepted shapes (no mixing):
//
//	{ }                              — empty
//	{ name: T, name: T }             — record (named fields, comma or semicolon)
//	{ [K]: V }                       — pure indexer
//	{ T }                            — array shorthand for { [number]: T }
func (p *Parser) parseTableType() ast.TypeNode {
	openTok := p.curToken
	p.nextToken() // consume '{'

	if p.curTokenIs(token.RBrace) {
		p.nextToken() // consume '}'
		return &ast.TypeTable{BaseNode: baseAt(openTok)}
	}

	var fields []ast.TypeTableField
	var indexer *ast.TypeIndexer

	// Detect array-shorthand `{ T }`: a single bare type with no `:`,
	// followed by `}`. We must distinguish this from `{ name: T }` (which
	// looks the same up to two tokens). The simplest disambiguator: peek
	// for `:` after the first identifier. If we see `Ident :`, it's a
	// named-field record. If we see `[`, it's an indexer. Otherwise treat
	// the contents as an array-shorthand T.
	switch {
	case p.curTokenIs(token.LBracket):
		// Pure indexer: `[K]: V`.
		p.nextToken() // consume '['
		key := p.parseType()
		if key == nil {
			return nil
		}
		if !p.expectCur(token.RBracket) {
			return nil
		}
		p.nextToken() // consume ']'
		if !p.expectCur(token.Colon) {
			return nil
		}
		p.nextToken() // consume ':'
		val := p.parseType()
		if val == nil {
			return nil
		}
		indexer = &ast.TypeIndexer{Key: key, Value: val}

	case p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon):
		// Named-field record. Read the first field; then loop on
		// `,`/`;`-separated additional fields.
		for {
			if !p.curTokenIs(token.Ident) {
				p.errorAt(p.curToken, errors.SyntaxError, "type",
					"expected field name in table type, got "+describeToken(p.curToken),
					"table types name each field: `{ x: number, y: number }`")
				return nil
			}
			name := p.curToken.Literal
			p.nextToken() // consume name
			if !p.expectCur(token.Colon) {
				return nil
			}
			p.nextToken() // consume ':'
			val := p.parseType()
			if val == nil {
				return nil
			}
			fields = append(fields, ast.TypeTableField{Key: name, Value: val})
			if !p.curTokenIs(token.Comma) && !p.curTokenIs(token.Semicolon) {
				break
			}
			p.nextToken() // consume separator
			// Trailing separator is allowed.
			if p.curTokenIs(token.RBrace) {
				break
			}
		}

	default:
		// Array shorthand `{ T }` — desugars to an indexer keyed by number.
		elem := p.parseType()
		if elem == nil {
			return nil
		}
		indexer = &ast.TypeIndexer{
			Key:   &ast.TypePrimitive{BaseNode: baseAt(openTok), Name: "number"},
			Value: elem,
		}
	}

	if !p.expectCur(token.RBrace) {
		return nil
	}
	p.nextToken() // consume '}'
	return &ast.TypeTable{BaseNode: baseAt(openTok), Fields: fields, Indexer: indexer}
}

// consumeTypeArgClose consumes the single `>` that closes a type-parameter
// or type-argument list. It transparently splits a `>>` (RShift) token into
// two `>` so nested generics like `Box<Box<number>>` parse: the first close
// rewrites the shared `>>` token in place to a lone `>` and does NOT advance,
// leaving the remaining `>` for the enclosing list to consume.
func (p *Parser) consumeTypeArgClose() bool {
	switch {
	case p.curTokenIs(token.GT):
		p.nextToken()
		return true
	case p.curTokenIs(token.RShift):
		p.curToken.Type = token.GT
		p.curToken.Literal = ">"
		return true
	default:
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected '>' to close type list, got "+describeToken(p.curToken),
			"type parameters and arguments are written between angle brackets: `Box<number>`")
		return false
	}
}

// parseTypeParamList reads a generic parameter declaration `<T, U>`. The
// cursor is on `<` on entry. Returns the parameter names; sets p.error and
// returns nil on failure. Bounds/defaults (`<T: number>`, `<T = number>`)
// are intentionally unsupported in v1.
func (p *Parser) parseTypeParamList() []string {
	p.nextToken() // consume '<'
	var params []string
	for {
		if !p.expectCur(token.Ident) {
			return nil
		}
		params = append(params, p.curToken.Literal)
		p.nextToken() // consume name
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken() // consume ','
	}
	if !p.consumeTypeArgClose() {
		return nil
	}
	return params
}

// parseTypeArgList reads a generic instantiation `<T1, T2>` in type position.
// The cursor is on `<` on entry. Returns the argument type nodes; sets
// p.error and returns nil on failure.
func (p *Parser) parseTypeArgList() []ast.TypeNode {
	p.nextToken() // consume '<'
	var args []ast.TypeNode
	for {
		a := p.parseType()
		if a == nil {
			return nil
		}
		args = append(args, a)
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken() // consume ','
	}
	if !p.consumeTypeArgClose() {
		return nil
	}
	return args
}

// isPrimitiveTypeName lists the closed set of Luau-style primitive type
// names. Anything else identifier-led is treated as a TypeName reference.
func isPrimitiveTypeName(s string) bool {
	switch s {
	case "number", "string", "boolean", "nil", "any", "unknown", "never":
		return true
	}
	return false
}
