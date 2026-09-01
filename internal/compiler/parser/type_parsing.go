package parser

import (
	"strconv"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

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
		p.nextToken()
		next := p.parseTypeAtom()
		if next == nil {
			return nil
		}
		members = append(members, next)
	}
	return &ast.TypeUnion{BaseNode: baseAt(startTok), Members: members}
}

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
		tok := p.curToken
		p.nextToken()
		t = &ast.TypePrimitive{BaseNode: baseAt(tok), Name: "nil"}
	case p.curTokenIs(token.String), p.curTokenIs(token.Int), p.curTokenIs(token.Float),
		p.curTokenIs(token.True), p.curTokenIs(token.False), p.curTokenIs(token.Minus):
		t = p.parseTypeLiteral()
	case p.curTokenIs(token.Ident):
		tok := p.curToken
		name := p.curToken.Literal
		p.nextToken()
		switch {
		case isPrimitiveTypeName(name):
			t = &ast.TypePrimitive{BaseNode: baseAt(tok), Name: name}
		case p.curTokenIs(token.LT):
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

	for p.curTokenIs(token.Question) {
		tok := p.curToken
		p.nextToken()
		t = &ast.TypeOptional{BaseNode: baseAt(tok), Inner: t}
	}
	return t
}

func (p *Parser) parseParenOrFunctionType() ast.TypeNode {
	openTok := p.curToken
	p.nextToken()

	var paramNames []string
	var paramTypes []ast.TypeNode
	isVararg := false
	var varargType ast.TypeNode

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
				break
			}

			name := ""
			if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon) {
				name = p.curToken.Literal
				p.nextToken()
				p.nextToken()
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
			p.nextToken()
		}
	}

	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken()

	if p.curTokenIs(token.Arrow) {
		p.nextToken()
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

	if isVararg || len(paramTypes) != 1 || (len(paramNames) > 0 && paramNames[0] != "") {
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected '->' after function-type parameter list",
			"function types are written `(<params>) -> <return>`, e.g. `(number, string) -> boolean`")
		return nil
	}
	return paramTypes[0]
}

func (p *Parser) parseReturnTypeList() []ast.TypeNode {
	if !p.curTokenIs(token.LParen) {
		t := p.parseType()
		if t == nil {
			return nil
		}
		return []ast.TypeNode{t}
	}
	p.nextToken()
	if p.curTokenIs(token.RParen) {
		p.nextToken()
		return nil
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
		p.nextToken()
	}
	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken()
	return rets
}

func (p *Parser) parseTableType() ast.TypeNode {
	openTok := p.curToken
	p.nextToken()

	if p.curTokenIs(token.RBrace) {
		p.nextToken()
		return &ast.TypeTable{BaseNode: baseAt(openTok)}
	}

	var fields []ast.TypeTableField
	var indexer *ast.TypeIndexer

	switch {
	case p.curTokenIs(token.LBracket):
		p.nextToken()
		key := p.parseType()
		if key == nil {
			return nil
		}
		if !p.expectCur(token.RBracket) {
			return nil
		}
		p.nextToken()
		if !p.expectCur(token.Colon) {
			return nil
		}
		p.nextToken()
		val := p.parseType()
		if val == nil {
			return nil
		}
		indexer = &ast.TypeIndexer{Key: key, Value: val}

	case p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon):
		for {
			if !p.curTokenIs(token.Ident) {
				p.errorAt(p.curToken, errors.SyntaxError, "type",
					"expected field name in table type, got "+describeToken(p.curToken),
					"table types name each field: `{ x: number, y: number }`")
				return nil
			}
			name := p.curToken.Literal
			p.nextToken()
			if !p.expectCur(token.Colon) {
				return nil
			}
			p.nextToken()
			val := p.parseType()
			if val == nil {
				return nil
			}
			fields = append(fields, ast.TypeTableField{Key: name, Value: val})
			if !p.curTokenIs(token.Comma) && !p.curTokenIs(token.Semicolon) {
				break
			}
			p.nextToken()
			if p.curTokenIs(token.RBrace) {
				break
			}
		}

	default:
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
	p.nextToken()
	return &ast.TypeTable{BaseNode: baseAt(openTok), Fields: fields, Indexer: indexer}
}

func (p *Parser) parseTypeArgs(name string, tok token.Token) ast.TypeNode {
	p.nextToken()
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
		p.nextToken()
	}
	if !p.closeTypeArg() {
		p.errorAt(p.curToken, errors.SyntaxError, "type",
			"expected '>' to close type arguments of '"+name+"', got "+describeToken(p.curToken),
			"generic instantiation looks like `Box<number>` or `Map<string, number>`")
		return nil
	}
	return &ast.TypeApplication{BaseNode: baseAt(tok), Name: name, Args: args}
}

func (p *Parser) closeTypeArg() bool {
	c := p.curToken
	switch c.Type {
	case token.GT:
		p.nextToken()
		return true
	case token.RShift:
		p.curToken = token.Token{Type: token.GT, Literal: ">", Line: c.Line, Column: c.Column + 1}
		return true
	case token.GTE:
		p.curToken = token.Token{Type: token.Assign, Literal: "=", Line: c.Line, Column: c.Column + 1}
		return true
	case token.RShiftAssign:
		p.curToken = token.Token{Type: token.GTE, Literal: ">=", Line: c.Line, Column: c.Column + 1}
		return true
	}
	return false
}

func (p *Parser) parseTypeParams() []string {
	if !p.curTokenIs(token.LT) {
		return nil
	}
	openTok := p.curToken
	p.nextToken()

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
		p.nextToken()
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken()
	}
	if !p.curTokenIs(token.GT) {
		p.errorAt(openTok, errors.SyntaxError, "type",
			"expected '>' to close the type-parameter list, got "+describeToken(p.curToken),
			"generic parameters look like `<T, U>`")
		return nil
	}
	p.nextToken()
	return params
}

func isPrimitiveTypeName(s string) bool {
	switch s {
	case "number", "string", "boolean", "nil", "any", "unknown", "never":
		return true
	}
	return false
}

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

	negate := false
	if p.curTokenIs(token.Minus) {
		negate = true
		p.nextToken()
		tok = p.curToken
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
