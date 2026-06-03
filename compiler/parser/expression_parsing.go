package parser

import (
	"fmt"

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/parser/errors"
	"github.com/hilthontt/sakura-lang/compiler/parser/precedence"
	"github.com/hilthontt/sakura-lang/compiler/token"
)

// ---------------------------------------------------------------------------
// Pratt-style expression parser.
//
// parseExpression(minPrec) reads the prefix (atom or unary) and then keeps
// folding infix operators of strictly higher precedence than `minPrec`.
// Right-associative operators (.. and ^) recurse on the RHS with `prec - 1`
// so an equal-precedence `^` will be absorbed instead of stopping the loop.
//
// Postfix forms — calls, method-calls, indexes, and table/string-call args —
// are folded by the same loop because they sit at the highest level (Call).
// ---------------------------------------------------------------------------

// parseExpression is the public entrypoint with the lowest precedence floor.
func (p *Parser) parseExpression() ast.Expression {
	return p.parseExpressionPrec(precedence.Lowest)
}

func (p *Parser) parseExpressionPrec(minPrec int) ast.Expression {
	left := p.parsePrefix()
	if left == nil {
		return nil
	}
	for {
		// Postfix call forms `f"str"` and `f{tbl}` start with a token that
		// has no infix entry but should still attach as a call at Call prec.
		if precedence.Call > minPrec && (p.curTokenIs(token.LBrace) || p.curTokenIs(token.String)) {
			left = p.parseCallWithSingleArg(left)
			continue
		}
		// `::` overlaps with goto-label statements (`::name::`). Peek
		// two tokens past the `::` — if the shape is `:: Ident ::` we're
		// at the head of a label statement, not a type assertion, so
		// stop expression parsing and let the statement dispatcher claim
		// the tokens.
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

// parsePrefix parses an atom or unary prefix expression.
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
	case token.String:
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

// parseInfix dispatches on the *current* token (which is the operator) and
// returns the folded expression. The caller has already consulted precedence
// and decided to fold, so this routine just reads the operator and the RHS.
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
		// Type assertion `expr :: T` — postfix at Call precedence so it
		// binds tightly enough to leave surrounding operators alone.
		return p.parseTypeAssertion(left)
	}
	return p.parseBinaryExpression(left, opPrec)
}

// parseTypeAssertion folds `:: T` onto an already-parsed expression. The
// runtime is unaffected; the type checker treats the result as T. Cursor
// is on `::` on entry.
func (p *Parser) parseTypeAssertion(left ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken() // consume '::'
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

// parseParenExpression handles `( exp )`. Lua uses parentheses to *adjust*
// a multi-value expression down to exactly one result, so the wrapper node
// is preserved (see ast.ParenExpression).
func (p *Parser) parseParenExpression() ast.Expression {
	openTok := p.curToken
	p.nextToken() // consume '('
	inner := p.parseExpression()
	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken() // consume ')'
	return &ast.ParenExpression{BaseNode: baseAt(openTok), Inner: inner}
}

func (p *Parser) parseUnaryExpression() ast.Expression {
	tok := p.curToken
	op := unaryOpString(tok.Type)
	p.nextToken()
	// Lua unary precedence binds tighter than every binary except ^,
	// which is right-associative and binds tighter still. Recursing at
	// `Unary` precedence handles both correctly.
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
		// Internal-shape error: parsePrecedence reached a non-operator token
		// when it expected an infix op. Surface enough context that the
		// user can find where their expression went off the rails.
		p.errorAt(tok, errors.SyntaxError, "",
			describeToken(tok)+" is not a binary operator",
			"this token can't combine two expressions; check for a missing operator or stray punctuation before it")
		return nil
	}
	p.nextToken() // consume operator
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
	p.nextToken() // consume '('
	args := []ast.Expression{}
	if !p.curTokenIs(token.RParen) {
		args = p.parseExpressionList()
	}
	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken() // consume ')'
	return &ast.CallExpression{BaseNode: baseAt(openTok), Func: callee, Args: args}
}

// parseCallWithSingleArg handles the sugar `f"str"` and `f{tbl}`. The current
// token is either `String` or `LBrace`.
func (p *Parser) parseCallWithSingleArg(callee ast.Expression) ast.Expression {
	tok := p.curToken
	var arg ast.Expression
	switch p.curToken.Type {
	case token.String:
		arg = &ast.StringLiteral{BaseNode: baseAt(tok), Value: tok.Literal}
		p.nextToken()
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
	p.nextToken() // consume '['
	idx := p.parseExpression()
	if !p.expectCur(token.RBracket) {
		return nil
	}
	p.nextToken() // consume ']'
	return &ast.IndexExpression{
		BaseNode: baseAt(tok),
		Object:   obj,
		Index:    idx,
		IsDot:    false,
	}
}

func (p *Parser) parseIndexDot(obj ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken() // consume '.'
	// Field names may use any identifier OR a soft/contextual keyword
	// (`match`). Hard keywords like `if`/`end` are still rejected.
	if !p.curTokenIsFieldName() {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
			"expected field name after '.', got "+describeToken(p.curToken),
			"")
		return nil
	}
	name := p.curToken.Literal
	nameTok := p.curToken
	p.nextToken() // consume Name
	return &ast.IndexExpression{
		BaseNode: baseAt(tok),
		Object:   obj,
		Index:    &ast.StringLiteral{BaseNode: baseAt(nameTok), Value: name},
		IsDot:    true,
	}
}

// curTokenIsFieldName reports whether the current token is acceptable as
// a field/method name (i.e. after '.' or ':'). Plain identifiers always
// qualify; contextual keywords (`match`) qualify because they are not
// reserved in expression positions.
func (p *Parser) curTokenIsFieldName() bool {
	return p.curTokenIs(token.Ident) || p.curTokenIs(token.Match)
}

// parseMethodCall handles `obj:method(args)` (and `obj:method"str"`,
// `obj:method{tbl}`). Lua's grammar requires `:` to be immediately followed
// by Name and then a call-args group.
func (p *Parser) parseMethodCall(obj ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken() // consume ':'
	if !p.curTokenIsFieldName() {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
			"expected method name after ':', got "+describeToken(p.curToken),
			"")
		return nil
	}
	method := p.curToken.Literal
	p.nextToken() // consume method name

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
	case token.String:
		strTok := p.curToken
		args = []ast.Expression{
			&ast.StringLiteral{BaseNode: baseAt(strTok), Value: strTok.Literal},
		}
		p.nextToken()
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

// parseFunctionExpression parses `function ( parlist ) block end`. The
// leading `function` keyword is the current token on entry.
func (p *Parser) parseFunctionExpression() ast.Expression {
	tok := p.curToken // 'function'
	p.nextToken()     // consume 'function'
	return p.parseFunctionBody(tok)
}

// parseFunctionBody handles `(parlist) block end`. `headerTok` is the
// position the resulting node should report (typically the `function`
// keyword, or for `local function f` the keyword `function` of the
// rewritten form).
func (p *Parser) parseFunctionBody(headerTok token.Token) *ast.FunctionExpression {
	if !p.expectCur(token.LParen) {
		return nil
	}
	p.nextToken() // consume '('

	params, isVararg, varargType := p.parseParamList()
	if p.error != nil {
		return nil
	}

	if !p.expectCur(token.RParen) {
		return nil
	}
	p.nextToken() // consume ')'

	// Optional Luau-style return-type annotation: `: T` or `: (T1, T2)`.
	var returnTypes []ast.TypeNode
	if p.curTokenIs(token.Colon) {
		p.nextToken() // consume ':'
		returnTypes = p.parseReturnTypeList()
		if p.error != nil {
			return nil
		}
	}

	// A function body opens a fresh loop scope: `break` inside the body
	// must NOT escape into a loop that encloses the function definition.
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
	p.nextToken() // consume 'end'

	return &ast.FunctionExpression{
		BaseNode:    baseAt(headerTok),
		Params:      params,
		IsVararg:    isVararg,
		VarargType:  varargType,
		ReturnTypes: returnTypes,
		Body:        body,
	}
}

// parseParamList reads `[ Param {, Param} [, Vararg] | Vararg ]` where
// Param = Name [: Type] and Vararg = '...' [: Type]. The current token on
// entry is the first parameter (or `)` for the empty list, which the caller
// short-circuits before calling here).
//
// Returns the param list, an IsVararg flag, and (for the typed-vararg case)
// the vararg's annotated type — nil when absent or unannotated.
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
		params = append(params, ast.TypedParam{Name: ident, Type: typ})
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken() // consume ','
		if p.curTokenIs(token.Vararg) {
			p.nextToken()
			return params, true, p.maybeParseColonType()
		}
	}
	return params, false, nil
}

// maybeParseColonType consumes `: T` if the cursor sits on `:`. Returns nil
// if there's no annotation, or on parser error.
func (p *Parser) maybeParseColonType() ast.TypeNode {
	if !p.curTokenIs(token.Colon) {
		return nil
	}
	p.nextToken() // consume ':'
	return p.parseType()
}

// parseTableConstructor reads `{ [field {fieldsep field} [fieldsep]] }`.
// The current token on entry is `{`.
func (p *Parser) parseTableConstructor() ast.Expression {
	tok := p.curToken
	p.nextToken() // consume '{'

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

		// fieldsep: ',' or ';'. Trailing one is allowed.
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
	p.nextToken() // consume '}'
	return tc
}

func (p *Parser) parseTableField() (ast.TableField, bool) {
	// `[exp] = exp` form
	if p.curTokenIs(token.LBracket) {
		p.nextToken() // consume '['
		key := p.parseExpression()
		if !p.expectCur(token.RBracket) {
			return ast.TableField{}, false
		}
		p.nextToken() // consume ']'
		if !p.expectCur(token.Assign) {
			return ast.TableField{}, false
		}
		p.nextToken() // consume '='
		val := p.parseExpression()
		return ast.TableField{Key: key, Value: val, IsBracketed: true}, true
	}
	// `Name = exp` form — only when curToken is Ident AND peek is '='.
	if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Assign) {
		nameTok := p.curToken
		key := &ast.Identifier{BaseNode: baseAt(nameTok), Name: nameTok.Literal}
		p.nextToken() // consume Name
		p.nextToken() // consume '='
		val := p.parseExpression()
		return ast.TableField{Key: key, Value: val}, true
	}
	// Otherwise positional: `exp`.
	val := p.parseExpression()
	if val == nil {
		return ast.TableField{}, false
	}
	return ast.TableField{Value: val}, true
}
