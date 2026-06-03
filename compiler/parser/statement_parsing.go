package parser

import (
	"fmt"

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/parser/errors"
	"github.com/hilthontt/sakura-lang/compiler/token"
)

// parseStatement dispatches on the current token to the matching statement
// parser. Lua's grammar allows several "expression-shaped" statements
// (assignment and function-call); these are disambiguated lazily by parsing
// a prefix expression and then peeking for `=` or `,`.
func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.Semicolon:
		// Bare `;` is a no-op; the block loop will consume it.
		p.nextToken()
		return nil

	case token.Local:
		return p.parseLocalStatement()
	case token.Function:
		return p.parseFunctionDeclaration()
	case token.If:
		return p.parseIfStatement()
	case token.While:
		return p.parseWhileStatement()
	case token.Repeat:
		return p.parseRepeatStatement()
	case token.For:
		return p.parseForStatement()
	case token.Do:
		return p.parseDoStatement()
	case token.Break:
		return p.parseBreakStatement()
	case token.Goto:
		return p.parseGotoStatement()
	case token.Label:
		return p.parseLabelStatement()
	case token.Match:
		return p.parseMatchStatement()
	case token.End, token.Else, token.ElseIf, token.Until:
		// These should be caught by parseBlock's loop; if we got here,
		// the surrounding block was malformed.
		p.errorAt(p.curToken, errors.UnexpectedEndError, "",
			"unexpected "+describeToken(p.curToken)+" with no matching block to close",
			"this keyword closes a block — make sure every `if`/`for`/`while`/`do`/`function`/`match` is properly opened first")
		return nil
	}

	// `type Name = T` — a type-alias statement. `type` is intentionally
	// NOT a reserved keyword (matching Luau) so existing code that uses
	// `type` as a variable name keeps compiling. The disambiguator is
	// peek == Ident, which is unambiguous: `type x` is meaningless as a
	// statement otherwise.
	if p.curTokenIs(token.Ident) && p.curToken.Literal == "type" && p.peekTokenIs(token.Ident) {
		return p.parseTypeAliasStatement()
	}

	// Otherwise: assignment or function-call statement.
	return p.parseExprOrAssignStatement()
}

// parseTypeAliasStatement reads `type Name = T`. The cursor is on the
// `type` identifier on entry.
func (p *Parser) parseTypeAliasStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'type'

	if !p.expectCur(token.Ident) {
		return nil
	}
	name := p.curToken.Literal
	p.nextToken() // consume Name

	if !p.expectCur(token.Assign) {
		return nil
	}
	p.nextToken() // consume '='

	target := p.parseType()
	if target == nil {
		return nil
	}
	return &ast.TypeAliasStatement{
		BaseNode: baseAt(tok),
		Name:     name,
		Target:   target,
	}
}

// parseReturnStatement reads `return [explist] [;]`. The caller must guard
// that `return` only appears as the last statement of a block.
func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	tok := p.curToken
	p.nextToken() // consume 'return'

	stmt := &ast.ReturnStatement{BaseNode: baseAt(tok)}
	// `return` can be followed by nothing (empty), a semicolon, or an
	// expression list that may itself be terminated by `;`.
	if p.endOfBlock() || p.curTokenIs(token.Semicolon) {
		return stmt
	}
	stmt.Values = p.parseExpressionList()
	return stmt
}

func (p *Parser) parseBreakStatement() ast.Statement {
	tok := p.curToken
	if p.loopDepth == 0 {
		p.errorAt(tok, errors.SyntaxError, "break",
			"'break' outside a loop",
			"break is only valid inside a for, while, or repeat loop")
		return nil
	}
	p.nextToken()
	return &ast.BreakStatement{BaseNode: baseAt(tok)}
}

func (p *Parser) parseGotoStatement() ast.Statement {
	tok := p.curToken
	if !p.expectPeek(token.Ident) {
		return nil
	}
	label := p.curToken.Literal
	p.nextToken()
	return &ast.GotoStatement{BaseNode: baseAt(tok), Label: label}
}

// parseLabelStatement reads `:: Name ::`. The opening `::` is the current
// token (lexer emits it as a single token of type `Label`).
func (p *Parser) parseLabelStatement() ast.Statement {
	tok := p.curToken
	if !p.expectPeek(token.Ident) {
		return nil
	}
	name := p.curToken.Literal
	if !p.expectPeek(token.Label) {
		return nil
	}
	p.nextToken() // consume closing '::'
	return &ast.LabelStatement{BaseNode: baseAt(tok), Name: name}
}

// parseLocalStatement handles both `local namelist [= explist]` and
// `local function Name funcbody`.
func (p *Parser) parseLocalStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'local'

	if p.curTokenIs(token.Function) {
		return p.parseLocalFunctionStatement(tok)
	}

	stmt := &ast.LocalStatement{BaseNode: baseAt(tok)}
	for {
		if !p.expectCur(token.Ident) {
			return nil
		}
		ln := ast.LocalName{Name: p.curToken.Literal}
		p.nextToken()
		// Optional Luau-style `: Type` annotation. Read BEFORE the
		// `<attrib>` block — `local x: T <const>` is the accepted order.
		if p.curTokenIs(token.Colon) {
			p.nextToken() // consume ':'
			ln.Type = p.parseType()
			if ln.Type == nil {
				return nil
			}
		}
		// Optional `<attrib>` — Lua 5.4 supports `<const>` and `<close>`.
		if p.curTokenIs(token.LT) {
			p.nextToken()
			if !p.expectCur(token.Ident) {
				return nil
			}
			ln.Attrib = p.curToken.Literal
			p.nextToken()
			if !p.expectCur(token.GT) {
				return nil
			}
			p.nextToken()
		}
		stmt.Names = append(stmt.Names, ln)
		if !p.curTokenIs(token.Comma) {
			break
		}
		p.nextToken()
	}

	if p.curTokenIs(token.Assign) {
		p.nextToken()
		stmt.Values = p.parseExpressionList()
	}
	return stmt
}

// parseLocalFunctionStatement handles `local function Name funcbody`. The
// opening `local` is at `localTok` and `curToken` is `function`.
func (p *Parser) parseLocalFunctionStatement(localTok token.Token) ast.Statement {
	fnTok := p.curToken // 'function'
	p.nextToken()       // consume 'function'
	if !p.expectCur(token.Ident) {
		return nil
	}
	name := p.curToken.Literal
	p.nextToken() // consume Name
	body := p.parseFunctionBody(fnTok)
	if body == nil {
		return nil
	}
	return &ast.LocalFunctionStatement{
		BaseNode: baseAt(localTok),
		Name:     name,
		Func:     body,
	}
}

// parseFunctionDeclaration handles `function funcname funcbody` where
// funcname is `Name {. Name} [: Name]`.
func (p *Parser) parseFunctionDeclaration() ast.Statement {
	tok := p.curToken // 'function'
	p.nextToken()     // consume 'function'

	if !p.expectCur(token.Ident) {
		return nil
	}
	name := &ast.Identifier{BaseNode: baseAt(p.curToken), Name: p.curToken.Literal}
	p.nextToken()

	var dotted []string
	var method string
	for p.curTokenIs(token.Dot) {
		p.nextToken()
		if !p.expectCur(token.Ident) {
			return nil
		}
		dotted = append(dotted, p.curToken.Literal)
		p.nextToken()
	}
	if p.curTokenIs(token.Colon) {
		p.nextToken()
		if !p.expectCur(token.Ident) {
			return nil
		}
		method = p.curToken.Literal
		p.nextToken()
	}

	body := p.parseFunctionBody(tok)
	if body == nil {
		return nil
	}
	return &ast.FunctionDeclaration{
		BaseNode:     baseAt(tok),
		Name:         name,
		DottedFields: dotted,
		MethodName:   method,
		Func:         body,
	}
}

func (p *Parser) parseIfStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'if'

	stmt := &ast.IfStatement{BaseNode: baseAt(tok)}

	cond := p.parseExpression()
	if !p.curTokenIs(token.Then) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "if",
			"expected 'then' after the condition, got "+describeToken(p.curToken),
			"syntax: `if <cond> then <body> [elseif ...] [else ...] end`")
		return nil
	}
	p.nextToken() // consume 'then'
	body := p.parseBlock()
	stmt.Clauses = append(stmt.Clauses, ast.IfClause{Condition: cond, Body: body})

	for p.curTokenIs(token.ElseIf) {
		elseifTok := p.curToken
		p.nextToken() // consume 'elseif'
		c := p.parseExpression()
		if !p.curTokenIs(token.Then) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "if",
				"expected 'then' after 'elseif' condition, got "+describeToken(p.curToken),
				fmt.Sprintf("the 'elseif' on line %d needs a 'then' before its body", elseifTok.Line))
			return nil
		}
		p.nextToken() // consume 'then'
		b := p.parseBlock()
		stmt.Clauses = append(stmt.Clauses, ast.IfClause{Condition: c, Body: b})
	}

	if p.curTokenIs(token.Else) {
		p.nextToken() // consume 'else'
		stmt.Else = p.parseBlock()
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "if",
			fmt.Sprintf("missing 'end' to close 'if' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'
	return stmt
}

func (p *Parser) parseWhileStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'while'

	cond := p.parseExpression()
	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "while",
			"expected 'do' after the condition, got "+describeToken(p.curToken),
			"syntax: `while <cond> do <body> end`")
		return nil
	}
	p.nextToken() // consume 'do'
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "while",
			fmt.Sprintf("missing 'end' to close 'while' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'
	return &ast.WhileStatement{BaseNode: baseAt(tok), Condition: cond, Body: body}
}

func (p *Parser) parseRepeatStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'repeat'

	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.Until) {
		p.errorAt(tok, errors.UnexpectedTokenError, "repeat",
			fmt.Sprintf("missing 'until' to close 'repeat' started on line %d, got %s",
				tok.Line, describeToken(p.curToken)),
			"syntax: `repeat <body> until <cond>` — the loop condition is checked AFTER the body")
		return nil
	}
	p.nextToken() // consume 'until'
	cond := p.parseExpression()
	return &ast.RepeatStatement{BaseNode: baseAt(tok), Body: body, Condition: cond}
}

func (p *Parser) parseDoStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'do'
	body := p.parseBlock()
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "do",
			fmt.Sprintf("missing 'end' to close 'do' block started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'
	return &ast.DoStatement{BaseNode: baseAt(tok), Body: body}
}

// parseForStatement disambiguates numeric vs generic for by looking at the
// token after the first Name: `=` → numeric; `,` or `in` → generic.
func (p *Parser) parseForStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume 'for'

	if !p.curTokenIs(token.Ident) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected loop-variable name after 'for', got "+describeToken(p.curToken),
			"syntax: `for i = 1, 10 do ... end` (numeric) or `for k, v in pairs(t) do ... end` (generic)")
		return nil
	}
	firstName := p.curToken.Literal
	p.nextToken()

	switch p.curToken.Type {
	case token.Assign:
		return p.parseNumericFor(tok, firstName)
	case token.Comma, token.In:
		return p.parseGenericFor(tok, firstName)
	}
	p.errorAt(p.curToken, errors.SyntaxError, "for",
		fmt.Sprintf("expected '=', ',', or 'in' after `for %s`, got %s",
			firstName, describeToken(p.curToken)),
		"use `for "+firstName+" = start, stop[, step]` for a numeric loop, or `for "+firstName+", ... in expr` for a generic one")
	return nil
}

func (p *Parser) parseNumericFor(tok token.Token, name string) ast.Statement {
	p.nextToken() // consume '='
	start := p.parseExpression()
	if !p.curTokenIs(token.Comma) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected ',' after the start expression in numeric 'for', got "+describeToken(p.curToken),
			"syntax: `for "+name+" = <start>, <stop>[, <step>] do ... end`")
		return nil
	}
	p.nextToken() // consume ','
	limit := p.parseExpression()
	var step ast.Expression
	if p.curTokenIs(token.Comma) {
		p.nextToken() // consume ','
		step = p.parseExpression()
	}
	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected 'do' after the numeric 'for' header, got "+describeToken(p.curToken),
			"syntax: `for "+name+" = <start>, <stop>[, <step>] do ... end`")
		return nil
	}
	p.nextToken() // consume 'do'
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "for",
			fmt.Sprintf("missing 'end' to close numeric 'for' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'
	return &ast.NumericForStatement{
		BaseNode: baseAt(tok),
		Name:     name,
		Start:    start,
		Limit:    limit,
		Step:     step,
		Body:     body,
	}
}

func (p *Parser) parseGenericFor(tok token.Token, firstName string) ast.Statement {
	names := []string{firstName}
	for p.curTokenIs(token.Comma) {
		p.nextToken() // consume ','
		if !p.curTokenIs(token.Ident) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
				"expected another loop-variable name after ',', got "+describeToken(p.curToken),
				"in a generic-for, all names go before `in`: `for k, v in pairs(t) do ... end`")
			return nil
		}
		names = append(names, p.curToken.Literal)
		p.nextToken()
	}
	if !p.curTokenIs(token.In) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected 'in' after generic-for variables, got "+describeToken(p.curToken),
			"syntax: `for k, v in pairs(t) do ... end`")
		return nil
	}
	p.nextToken() // consume 'in'
	exprs := p.parseExpressionList()
	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected 'do' after the generic-for expression, got "+describeToken(p.curToken),
			"syntax: `for k, v in pairs(t) do ... end`")
		return nil
	}
	p.nextToken() // consume 'do'
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "for",
			fmt.Sprintf("missing 'end' to close generic 'for' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'
	return &ast.GenericForStatement{
		BaseNode: baseAt(tok),
		Names:    names,
		Exprs:    exprs,
		Body:     body,
	}
}

// parseExprOrAssignStatement reads a prefix expression. If it's followed by
// `=` or `,` we have an assignment; otherwise it must be a call (the only
// expression Lua allows in statement position).
func (p *Parser) parseExprOrAssignStatement() ast.Statement {
	tok := p.curToken
	first := p.parseExpression()
	if first == nil {
		return nil
	}

	if op, ok := compoundOps[p.curToken.Type]; ok {
		return p.parseCompoundAssignStatement(tok, first, op)
	}
	if p.curTokenIs(token.Assign) || p.curTokenIs(token.Comma) {
		return p.parseAssignmentStatement(tok, first)
	}

	// Statement-position expression: must be a call.
	switch first.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression:
		return &ast.ExpressionStatement{BaseNode: baseAt(tok), Expression: first}
	}
	p.errorAt(tok, errors.SyntaxError, "",
		fmt.Sprintf("expression %q is not a valid statement", first.String()),
		"Lua only allows function calls and assignments at statement position; did you mean `local x = ...` or `x = ...`?")
	return nil
}

// parseCompoundAssignStatement desugars `target op= rhs` into a regular
// AssignStatement of the form `target = target op rhs`. The cursor on
// entry is on the compound operator token; `binOp` is the binary operator
// string (e.g. "+", "<<") to use in the synthesised BinaryExpression.
//
// Caveat: for IndexExpression targets, this duplicates the object and
// key expressions. If those have side effects (e.g. `t[f()] += 1`),
// they will be evaluated twice. Acceptable for v1 — matches Luau's
// own initial implementation.
func (p *Parser) parseCompoundAssignStatement(tok token.Token, target ast.Expression, binOp string) ast.Statement {
	if !isAssignTarget(target) {
		p.errorAt(tok, errors.InvalidAssignmentError, "",
			fmt.Sprintf("invalid compound-assignment target %q", target.String()),
			"the LHS of `op=` must be a name, a field access (t.x), or an index (t[k])")
		return nil
	}
	opTok := p.curToken
	p.nextToken() // consume the compound op
	rhs := p.parseExpression()
	if rhs == nil {
		return nil
	}
	combined := &ast.BinaryExpression{
		BaseNode: baseAt(opTok),
		Op:       binOp,
		Left:     target,
		Right:    rhs,
	}
	return &ast.AssignStatement{
		BaseNode: baseAt(tok),
		Targets:  []ast.Expression{target},
		Values:   []ast.Expression{combined},
	}
}

func (p *Parser) parseAssignmentStatement(tok token.Token, first ast.Expression) ast.Statement {
	if !isAssignTarget(first) {
		p.errorAt(tok, errors.InvalidAssignmentError, "",
			fmt.Sprintf("invalid assignment target %q", first.String()),
			"the LHS of `=` must be a name, a field access (t.x), or an index (t[k])")
		return nil
	}
	targets := []ast.Expression{first}
	for p.curTokenIs(token.Comma) {
		p.nextToken() // consume ','
		nxt := p.parseExpression()
		if nxt == nil {
			return nil
		}
		if !isAssignTarget(nxt) {
			p.errorAt(tok, errors.InvalidAssignmentError, "",
				fmt.Sprintf("invalid assignment target %q in multi-assignment", nxt.String()),
				"every LHS target must be a name, a field access (t.x), or an index (t[k])")
			return nil
		}
		targets = append(targets, nxt)
	}
	if !p.curTokenIs(token.Assign) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "",
			"expected '=' after assignment targets, got "+describeToken(p.curToken),
			"syntax: `a, b, c = expr1, expr2, expr3`")
		return nil
	}
	p.nextToken() // consume '='
	values := p.parseExpressionList()
	return &ast.AssignStatement{
		BaseNode: baseAt(tok),
		Targets:  targets,
		Values:   values,
	}
}

// isAssignTarget reports whether an expression is a valid LHS target. Lua
// permits Name, prefixexp[exp], and prefixexp.Name.
func isAssignTarget(e ast.Expression) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.IndexExpression:
		return true
	}
	return false
}
