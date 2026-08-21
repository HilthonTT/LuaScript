package parser

import (
	"strconv"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

// parseType is the entry point for a type expression. It handles `|` unions
// left-to-right around parseTypeAtom (which itself handles the `?` postfix).
//
//	T   :=  Atom { '|' Atom }
//	Atom := Primitive | Literal | Name | '(' ... ')' [ '->' Returns ] | TableType
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
	if p.enterDepth("type") {
		return nil
	}
	defer p.leaveDepth()

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
	case p.curTokenIs(token.String), p.curTokenIs(token.Int), p.curTokenIs(token.Float),
		p.curTokenIs(token.True), p.curTokenIs(token.False), p.curTokenIs(token.Minus):
		// Singleton (literal) type: `"read"`, `42`, `-1`, `true`.
		t = p.parseTypeLiteral()
	case p.curTokenIs(token.Ident):
		// Identifier-led atom: distinguish built-in primitives from user
		// alias references purely by name. The set of primitives is closed.
		tok := p.curToken
		name := p.curToken.Literal
		p.nextToken()
		switch {
		case isPrimitiveTypeName(name):
			t = &ast.TypePrimitive{BaseNode: baseAt(tok), Name: name}
		case p.curTokenIs(token.LT):
			// Generic instantiation `Name<A, B>`. In type position `<` is
			// never a comparison, so this is unambiguous.
			t = p.parseTypeArgs(name, tok)
			if t == nil {
				return nil
			}
		default:
			t = &ast.TypeName{BaseNode: baseAt(tok), Name: name}
		}
	default:
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected a type, got "+describeToken(p.curToken),
			"valid types: a name (`number`, `MyAlias`), a literal (`\"read\"`, `42`, `true`), a function type `(A) -> B`, a table `{ x: T }`, or a union `A | B`")
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

// parseTypeArgs reads the `< A, B, ... >` argument list of a generic
// instantiation `Name<...>`. The cursor is on `<` at entry. Returns a
// TypeApplication, or nil on error.
func (p *Parser) parseTypeArgs(name string, tok token.Token) ast.TypeNode {
	p.nextToken() // consume '<'
	var args []ast.TypeNode
	for {
		arg := p.parseType()
		if arg == nil {
			return nil
		}
		args = append(args, arg)
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken() // consume ','
	}
	if !p.closeTypeArg() {
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected '>' to close type arguments of '"+name+"', got "+describeToken(p.curToken),
			"generic instantiation looks like `Box<number>` or `Map<string, number>`")
		return nil
	}
	return &ast.TypeApplication{BaseNode: baseAt(tok), Name: name, Args: args}
}

// closeTypeArg consumes exactly one `>` that closes a type-argument list. To
// support nested generics like `Array<Array<number>>`, a compound token whose
// first character is `>` (`>>`, `>=`, `>>=`) is split: one `>` is consumed and
// the remainder is left as the current token for the enclosing level (or the
// following statement). Returns false when the current token is not a `>`-led
// closer.
func (p *Parser) closeTypeArg() bool {
	c := p.curToken
	switch c.Type {
	case token.GT:
		p.nextToken()
		return true
	case token.RShift: // '>>' -> consume one '>', leave '>'
		p.curToken = token.Token{Type: token.GT, Literal: ">", Line: c.Line, Column: c.Column + 1}
		return true
	case token.GTE: // '>=' -> consume '>', leave '='
		p.curToken = token.Token{Type: token.Assign, Literal: "=", Line: c.Line, Column: c.Column + 1}
		return true
	case token.RShiftAssign: // '>>=' -> consume '>', leave '>='
		p.curToken = token.Token{Type: token.GTE, Literal: ">=", Line: c.Line, Column: c.Column + 1}
		return true
	}
	return false
}

// parseTypeParams reads an optional generic parameter list `< T, U, ... >`
// and returns the parameter names. When the cursor is not on `<` it returns
// nil and leaves the cursor untouched — callers use this to make the list
// optional. Duplicate names are reported. Shared by struct, function, and
// type-alias declarations.
func (p *Parser) parseTypeParams() []string {
	if !p.curTokenIs(token.LT) {
		return nil
	}
	openTok := p.curToken
	p.nextToken() // consume '<'

	var params []string
	seen := map[string]bool{}
	for {
		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.SyntaxError, "type",
				"expected a type-parameter name, got "+describeToken(p.curToken),
				"generic parameters are names: `<T>`, `<K, V>`")
			return nil
		}
		name := p.curToken.Literal
		if seen[name] {
			p.errorAt(p.curToken, errors.SyntaxError, "type",
				"duplicate type parameter '"+name+"'",
				"each type parameter in the list must be unique")
			return nil
		}
		seen[name] = true
		params = append(params, name)
		p.nextToken() // consume name
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken() // consume ','
	}
	if !p.curTokenIs(token.GT) {
		p.errorAt(openTok, errors.SyntaxError, "type",
			"expected '>' to close the type-parameter list, got "+describeToken(p.curToken),
			"generic parameters look like `<T, U>`")
		return nil
	}
	p.nextToken() // consume '>'
	return params
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

// parseTypeLiteral parses a singleton type: a string, number (with optional
// leading `-`), or boolean literal appearing in type position. It reuses the
// expression-level literal parsers so type-position numbers accept exactly
// the syntax expression-position numbers do (hex, floats, exponents) and
// convert identically. Returns nil and records an error on failure.
func (p *Parser) parseTypeLiteral() ast.TypeNode {
	tok := p.curToken

	switch {
	case p.curTokenIs(token.String):
		p.nextToken()
		return &ast.TypeLiteral{
			BaseNode: baseAt(tok),
			Kind:     ast.LiteralString,
			Str:      tok.Literal,
			Raw:      strconv.Quote(tok.Literal),
		}

	case p.curTokenIs(token.True), p.curTokenIs(token.False):
		v := p.curTokenIs(token.True)
		p.nextToken()
		return &ast.TypeLiteral{
			BaseNode: baseAt(tok),
			Kind:     ast.LiteralBoolean,
			Bool:     v,
			Raw:      strconv.FormatBool(v),
		}
	}

	// Numeric, possibly negated. `-` binds only to a number here; there is no
	// other prefix operator in type position.
	negate := false
	if p.curTokenIs(token.Minus) {
		negate = true
		p.nextToken()
		tok = p.curToken // the number itself, for Raw and position
		if !p.curTokenIs(token.Int) && !p.curTokenIs(token.Float) {
			p.errorAt(p.curToken, errors.SyntaxError, "type",
				"expected a number after '-' in a literal type, got "+describeToken(p.curToken),
				"negative literal types look like `-1`")
			return nil
		}
	}

	var expr ast.Expression
	if p.curTokenIs(token.Int) {
		expr = p.parseIntegerLiteral()
	} else {
		expr = p.parseFloatLiteral()
	}
	if expr == nil {
		return nil
	}

	var num float64
	switch n := expr.(type) {
	case *ast.IntegerLiteral:
		num = float64(n.Value)
	case *ast.FloatLiteral:
		num = n.Value
	default:
		return nil
	}
	raw := tok.Literal
	if negate {
		num = -num
		raw = "-" + raw
	}
	return &ast.TypeLiteral{
		BaseNode: baseAt(tok),
		Kind:     ast.LiteralNumber,
		Num:      num,
		Raw:      raw,
	}
}
