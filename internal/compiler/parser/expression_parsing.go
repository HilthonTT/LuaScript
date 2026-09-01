package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/parser/precedence"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func (p *Parser) parseExpression() ast.Expression {
	return p.parseExpressionPrec(precedence.Lowest)
}

func (p *Parser) parseExpressionPrec(minPrec int) ast.Expression {
	if p.enterDepth("expression") {
		return nil
	}
	defer p.leaveDepth()
	left := p.parsePrefix()
	if left == nil {
		return nil
	}
	for {
		if precedence.Call > minPrec && (p.curTokenIs(token.LBrace) || p.curTokenIs(token.String) || p.curTokenIs(token.InterpString)) {
			left = p.parseCallWithSingleArg(left)
			continue
		}
		if p.curTokenIs(token.Label) && p.peekTokenIs(token.Ident) && p.peek2Token().Type == token.Label {
			break
		}
		curPrec, ok := precedence.LookupTable[p.curToken.Type]
		if !ok || curPrec <= minPrec {
			break
		}
		left = p.parseInfix(left, curPrec)
		if left == nil {
			return nil
		}
	}
	return left
}

func (p *Parser) parsePrefix() ast.Expression {
	switch p.curToken.Type {
	case token.Nil:
		exp := &ast.NilLiteral{BaseNode: baseAt(p.curToken)}
		p.nextToken()
		return exp
	case token.True:
		return p.parseTrueLiteral()
	case token.False:
		return p.parseFalseLiteral()
	case token.Int:
		return p.parseIntegerLiteral()
	case token.Float:
		return p.parseFloatLiteral()
	case token.String, token.InterpString:
		return p.parseStringLiteral()
	case token.Vararg:
		return p.parseVarArg()
	case token.Ident:
		return p.parseIdent()
	case token.LParen:
		return p.parseParenExpression()
	case token.LBrace:
		return p.parseTableConstructor()
	case token.Function:
		return p.parseFunctionExpression()
	case token.If:
		return p.parseIfExpression()
	case token.Minus, token.Not, token.Hash, token.Tilde:
		return p.parseUnaryExpression()
	}

	hint := ""
	switch p.curToken.Type {
	case token.End, token.Then, token.Else, token.ElseIf, token.Until, token.Do:
		hint = "this keyword closes a block — an expression appears to be missing before it"
	case token.RParen, token.RBracket, token.RBrace:
		hint = "stray closing " + describeToken(p.curToken) + " — check earlier delimiters for a missing opener"
	case token.Comma:
		hint = "stray ',' — did you finish writing the previous expression?"
	case token.Assign:
		hint = "'=' is assignment, not equality; use '==' for comparison"
	case token.EOF:
		hint = "the source ends here while an expression was expected"
	}
	p.errorAt(p.curToken, errors.SyntaxError, "",
		"unexpected "+describeToken(p.curToken)+" at start of expression",
		hint)
	return nil
}

func (p *Parser) parseInfix(left ast.Expression, opPrec int) ast.Expression {
	switch p.curToken.Type {
	case token.LParen:
		return p.parseCall(left)
	case token.LBracket:
		return p.parseIndexBracket(left)
	case token.Dot:
		return p.parseIndexDot(left)
	case token.Colon:
		return p.parseMethodCall(left)
	case token.Label:
		return p.parseTypeAssertion(left)
	}
	return p.parseBinaryExpression(left, opPrec)
}

func (p *Parser) parseTypeAssertion(left ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken()
	t := p.parseType()
	if t == nil {
		return nil
	}
	return &ast.TypeAssertionExpression{
		BaseNode: baseAt(tok),
		Expr:     left,
		Type:     t,
	}
}

func (p *Parser) parseIfExpression() ast.Expression {
	tok := p.curToken
	p.nextToken()

	expr := &ast.IfExpression{BaseNode: baseAt(tok)}
	for {
		cond := p.parseExpression()
		if cond == nil {
			return nil
		}
		if !p.curTokenIs(token.Then) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "if expression",
				"expected 'then' after the condition, got "+describeToken(p.curToken),
				"syntax: `if <cond> then <value> else <value>` (no `end`)")
			return nil
		}
		p.nextToken()
		val := p.parseExpression()
		if val == nil {
			return nil
		}
		expr.Clauses = append(expr.Clauses, ast.IfExprClause{Condition: cond, Value: val})

		if p.curTokenIs(token.ElseIf) {
			p.nextToken()
			continue
		}
		break
	}
	if !p.curTokenIs(token.Else) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "if expression",
			"expected 'else' to complete the if expression, got "+describeToken(p.curToken),
			"an if expression must always produce a value, so the `else` arm is mandatory")
		return nil
	}
	p.nextToken()
	expr.Else = p.parseExpression()
	if expr.Else == nil {
		return nil
	}
	return expr
}

func (p *Parser) parseParenExpression() ast.Expression {
	openTok := p.curToken
	p.nextToken()
	inner := p.parseExpression()
	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken()
	return &ast.ParenExpression{BaseNode: baseAt(openTok), Inner: inner}
}

func (p *Parser) parseUnaryExpression() ast.Expression {
	tok := p.curToken
	op := unaryOpString(tok.Type)
	p.nextToken()
	operand := p.parseExpressionPrec(precedence.Unary)
	if operand == nil {
		return nil
	}
	return &ast.UnaryExpression{BaseNode: baseAt(tok), Op: op, Operand: operand}
}

func unaryOpString(t token.Type) string {
	switch t {
	case token.Minus:
		return "-"
	case token.Not:
		return "not"
	case token.Hash:
		return "#"
	case token.Tilde:
		return "~"
	}
	return ""
}

func (p *Parser) parseBinaryExpression(left ast.Expression, opPrec int) ast.Expression {
	tok := p.curToken
	op := binaryOpString(tok.Type)
	if op == "" {
		p.errorAt(tok, errors.SyntaxError, "",
			describeToken(tok)+" is not a binary operator",
			"this token can't combine two expressions; check for a missing operator or stray punctuation before it")
		return nil
	}
	p.nextToken()
	rhsPrec := opPrec
	if precedence.IsRightAssoc(tok.Type) {
		rhsPrec = opPrec - 1
	}
	right := p.parseExpressionPrec(rhsPrec)
	if right == nil {
		return nil
	}
	return &ast.BinaryExpression{
		BaseNode: baseAt(tok),
		Op:       op,
		Left:     left,
		Right:    right,
	}
}

func binaryOpString(t token.Type) string {
	switch t {
	case token.Plus:
		return "+"
	case token.Minus:
		return "-"
	case token.Asterisk:
		return "*"
	case token.Slash:
		return "/"
	case token.FloorDiv:
		return "//"
	case token.Percent:
		return "%"
	case token.Caret:
		return "^"
	case token.Concat:
		return ".."
	case token.Eq:
		return "=="
	case token.NotEq:
		return "~="
	case token.LT:
		return "<"
	case token.LTE:
		return "<="
	case token.GT:
		return ">"
	case token.GTE:
		return ">="
	case token.Ampersand:
		return "&"
	case token.Pipe:
		return "|"
	case token.Tilde:
		return "~"
	case token.LShift:
		return "<<"
	case token.RShift:
		return ">>"
	case token.And:
		return "and"
	case token.Or:
		return "or"
	}
	return ""
}

func (p *Parser) parseCall(callee ast.Expression) ast.Expression {
	openTok := p.curToken
	p.nextToken()
	args := []ast.Expression{}
	if !p.curTokenIs(token.RParen) {
		args = p.parseExpressionList()
	}
	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken()
	return &ast.CallExpression{BaseNode: baseAt(openTok), Func: callee, Args: args}
}

func (p *Parser) parseCallWithSingleArg(callee ast.Expression) ast.Expression {
	tok := p.curToken
	var arg ast.Expression
	switch p.curToken.Type {
	case token.String, token.InterpString:
		arg = p.parseStringLiteral()
	case token.LBrace:
		arg = p.parseTableConstructor()
	}
	if arg == nil {
		return nil
	}
	return &ast.CallExpression{BaseNode: baseAt(tok), Func: callee, Args: []ast.Expression{arg}}
}

func (p *Parser) parseIndexBracket(obj ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken()
	idx := p.parseExpression()
	if !p.expectCur(token.RBracket) {
		return nil
	}
	p.nextToken()
	return &ast.IndexExpression{
		BaseNode: baseAt(tok),
		Object:   obj,
		Index:    idx,
		IsDot:    false,
	}
}

func (p *Parser) parseIndexDot(obj ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken()
	if !p.curTokenIsFieldName() {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
			"expected field name after '.', got "+describeToken(p.curToken),
			"")
		return nil
	}
	name := p.curToken.Literal
	nameTok := p.curToken
	p.nextToken()
	return &ast.IndexExpression{
		BaseNode: baseAt(tok),
		Object:   obj,
		Index:    &ast.StringLiteral{BaseNode: baseAt(nameTok), Value: name},
		IsDot:    true,
	}
}

func (p *Parser) curTokenIsFieldName() bool {
	return p.curTokenIs(token.Ident) || p.curTokenIs(token.Match)
}

func (p *Parser) parseMethodCall(obj ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken()
	if !p.curTokenIsFieldName() {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
			"expected method name after ':', got "+describeToken(p.curToken),
			"")
		return nil
	}
	method := p.curToken.Literal
	p.nextToken()

	args := []ast.Expression{}
	switch p.curToken.Type {
	case token.LParen:
		p.nextToken()
		if !p.curTokenIs(token.RParen) {
			args = p.parseExpressionList()
		}
		if !p.expectCur(token.RParen) {
			return nil
		}
		p.nextToken()
	case token.String, token.InterpString:
		args = []ast.Expression{p.parseStringLiteral()}
	case token.LBrace:
		tbl := p.parseTableConstructor()
		if tbl == nil {
			return nil
		}
		args = []ast.Expression{tbl}
	default:
		p.errorAt(p.curToken, errors.SyntaxError, "",
			fmt.Sprintf("expected call arguments after `:%s`, got %s", method, describeToken(p.curToken)),
			"a method call needs arguments: `obj:"+method+"(...)`, `obj:"+method+"\"str\"`, or `obj:"+method+"{tbl}`")
		return nil
	}

	return &ast.MethodCallExpression{
		BaseNode: baseAt(tok),
		Object:   obj,
		Method:   method,
		Args:     args,
	}
}

func (p *Parser) parseExpressionList() []ast.Expression {
	exprs := []ast.Expression{p.parseExpression()}
	for p.curTokenIs(token.Comma) {
		p.nextToken()
		exprs = append(exprs, p.parseExpression())
	}
	return exprs
}

func (p *Parser) parseFunctionExpression() ast.Expression {
	tok := p.curToken
	p.nextToken()
	return p.parseFunctionBody(tok)
}

func (p *Parser) parseFunctionBody(headerTok token.Token) *ast.FunctionExpression {
	var typeParams []string
	if p.curTokenIs(token.LT) {
		typeParams = p.parseTypeParams()
		if p.error != nil {
			return nil
		}
	}

	if !p.expectCur(token.LParen) {
		return nil
	}
	p.nextToken()

	params, isVararg, varargType := p.parseParamList()
	if p.error != nil {
		return nil
	}

	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken()

	var returnTypes []ast.TypeNode
	if p.curTokenIs(token.Colon) {
		p.nextToken()
		returnTypes = p.parseReturnTypeList()
		if p.error != nil {
			return nil
		}
	}

	savedLoopDepth := p.loopDepth
	p.loopDepth = 0
	body := p.parseBlock()
	p.loopDepth = savedLoopDepth
	if p.error != nil {
		return nil
	}
	if !p.expectCur(token.End) {
		return nil
	}
	p.nextToken()

	return &ast.FunctionExpression{
		BaseNode:    baseAt(headerTok),
		TypeParams:  typeParams,
		Params:      params,
		IsVararg:    isVararg,
		VarargType:  varargType,
		ReturnTypes: returnTypes,
		Body:        body,
	}
}

func (p *Parser) parseParamList() ([]ast.TypedParam, bool, ast.TypeNode) {
	if p.curTokenIs(token.RParen) {
		return nil, false, nil
	}
	if p.curTokenIs(token.Vararg) {
		p.nextToken()
		return nil, true, p.maybeParseColonType()
	}
	params := []ast.TypedParam{}
	for {
		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.SyntaxError, "function",
				"expected parameter name, got "+describeToken(p.curToken),
				"function parameters are bare names: `function f(a, b: number, ...) ... end`")
			return nil, false, nil
		}
		ident := &ast.Identifier{
			BaseNode: baseAt(p.curToken),
			Name:     p.curToken.Literal,
		}
		p.nextToken()
		typ := p.maybeParseColonType()
		if p.error != nil {
			return nil, false, nil
		}
		var def ast.Expression
		if p.curTokenIs(token.Assign) {
			p.nextToken()
			def = p.parseExpression()
			if def == nil {
				return nil, false, nil
			}
		}
		params = append(params, ast.TypedParam{Name: ident, Type: typ, Default: def})
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken()
		if p.curTokenIs(token.Vararg) {
			p.nextToken()
			return params, true, p.maybeParseColonType()
		}
	}
	return params, false, nil
}

func (p *Parser) maybeParseColonType() ast.TypeNode {
	if !p.curTokenIs(token.Colon) {
		return nil
	}
	p.nextToken()
	return p.parseType()
}

func (p *Parser) parseTableConstructor() ast.Expression {
	tok := p.curToken
	p.nextToken()

	tc := &ast.TableConstructor{BaseNode: baseAt(tok)}
	if p.curTokenIs(token.RBrace) {
		p.nextToken()
		return tc
	}

	for {
		field, ok := p.parseTableField()
		if !ok {
			return nil
		}
		tc.Fields = append(tc.Fields, field)

		if p.curTokenIs(token.Comma) || p.curTokenIs(token.Semicolon) {
			p.nextToken()
			if p.curTokenIs(token.RBrace) {
				break
			}
			continue
		}
		break
	}
	if !p.expectCur(token.RBrace) {
		return nil
	}
	p.nextToken()
	return tc
}

func (p *Parser) parseTableField() (ast.TableField, bool) {
	if p.curTokenIs(token.LBracket) {
		p.nextToken()
		key := p.parseExpression()
		if !p.expectCur(token.RBracket) {
			return ast.TableField{}, false
		}
		p.nextToken()
		if !p.expectCur(token.Assign) {
			return ast.TableField{}, false
		}
		p.nextToken()
		val := p.parseExpression()
		return ast.TableField{Key: key, Value: val, IsBracketed: true}, true
	}
	if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Assign) {
		nameTok := p.curToken
		key := &ast.Identifier{BaseNode: baseAt(nameTok), Name: nameTok.Literal}
		p.nextToken()
		p.nextToken()
		val := p.parseExpression()
		return ast.TableField{Key: key, Value: val}, true
	}
	val := p.parseExpression()
	if val == nil {
		return ast.TableField{}, false
	}
	return ast.TableField{Value: val}, true
}
