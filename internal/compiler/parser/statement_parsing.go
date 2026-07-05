package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
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
	case token.Enum:
		return p.parseEnumStatement()
	case token.Defer:
		return p.parseDeferStatement()
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

	// `struct Name { ... }` — a nominal product-type declaration. Like
	// `type`, `struct` is a *soft* keyword (not reserved) so existing code
	// using `struct` as a variable keeps compiling. The disambiguator is
	// `struct <Ident>`, which is otherwise not a valid statement start
	// (two bare identifiers in a row is a syntax error in Lua).
	if p.curTokenIs(token.Ident) && p.curToken.Literal == "struct" && p.peekTokenIs(token.Ident) {
		return p.parseStructStatement()
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

	// Optional generic parameters: `type Box<T> = ...`.
	var typeParams []string
	if p.curTokenIs(token.LT) {
		typeParams = p.parseTypeParams()
		if p.error != nil {
			return nil
		}
	}

	if !p.expectCur(token.Assign) {
		return nil
	}
	p.nextToken() // consume '='

	target := p.parseType()
	if target == nil {
		return nil
	}
	return &ast.TypeAliasStatement{
		BaseNode:   baseAt(tok),
		Name:       name,
		TypeParams: typeParams,
		Target:     target,
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

// parseCompoundAssignStatement desugars `target op= rhs` into `target =
// target op rhs`. The cursor on entry is on the compound operator token;
// `binOp` is the binary operator string (e.g. "+", "<<").
//
// For an index target `t[k] op= rhs`, a naive desugar duplicates the object
// and key subtrees, so side effects in them (`t[f()] += 1`) would run twice.
// To evaluate them exactly once, an index target is hoisted into fresh locals
// inside a scoping `do` block:
//
//	do local __caobj_N = t; local __cakey_N = f(); __caobj_N[__cakey_N] = __caobj_N[__cakey_N] op rhs end
//
// The dot form `t.x` keeps its constant string key un-hoisted (no side effect).
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

	// mkAssign builds `lhsTarget = lhsRead op rhs`. lhsTarget and lhsRead must
	// be independent AST nodes (they land in different positions of the tree).
	mkAssign := func(lhsTarget, lhsRead ast.Expression) ast.Statement {
		return &ast.AssignStatement{
			BaseNode: baseAt(tok),
			Targets:  []ast.Expression{lhsTarget},
			Values: []ast.Expression{&ast.BinaryExpression{
				BaseNode: baseAt(opTok),
				Op:       binOp,
				Left:     lhsRead,
				Right:    rhs,
			}},
		}
	}

	idx, ok := target.(*ast.IndexExpression)
	if !ok {
		// Name target: nothing to duplicate, reuse the node on both sides.
		return mkAssign(target, target)
	}

	// Index target: hoist object (and key, unless it's a constant dot field).
	p.compoundCounter++
	objName := fmt.Sprintf("__caobj_%d", p.compoundCounter)
	body := &ast.Block{
		BaseNode: baseAt(tok),
		Statements: []ast.Statement{&ast.LocalStatement{
			BaseNode: baseAt(tok),
			Names:    []ast.LocalName{{Name: objName}},
			Values:   []ast.Expression{idx.Object},
		}},
	}
	newIndex := func() ast.Expression {
		key := idx.Index
		if !idx.IsDot {
			key = &ast.Identifier{BaseNode: baseAt(tok), Name: fmt.Sprintf("__cakey_%d", p.compoundCounter)}
		}
		return &ast.IndexExpression{
			BaseNode: baseAt(tok),
			Object:   &ast.Identifier{BaseNode: baseAt(tok), Name: objName},
			Index:    key,
			IsDot:    idx.IsDot,
		}
	}
	if !idx.IsDot {
		body.Statements = append(body.Statements, &ast.LocalStatement{
			BaseNode: baseAt(tok),
			Names:    []ast.LocalName{{Name: fmt.Sprintf("__cakey_%d", p.compoundCounter)}},
			Values:   []ast.Expression{idx.Index},
		})
	}
	body.Statements = append(body.Statements, mkAssign(newIndex(), newIndex()))
	return &ast.DoStatement{BaseNode: baseAt(tok), Body: body}
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

// parseEnumStatement consumes
//
//	enum Name
//	    VARIANT,
//	    VARIANT,
//	    ...
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

// parseDeferStatement reads `defer <call>`. Like Go's defer it accepts only a
// function or method call. The cursor is on `defer` at entry; whatever follows
// the call (including a trailing `;`) is left for the block loop to consume.
func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	deferTok := p.curToken
	p.nextToken() // consume 'defer'

	call := p.parseExpression()
	if call == nil {
		return nil
	}
	switch call.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression:
	default:
		p.errorAt(deferTok, errors.SyntaxError, "defer",
			"defer expects a function or method call",
			"syntax: defer cleanup(args)  or  defer obj:method(args)")
		return nil
	}
	return &ast.DeferStatement{BaseNode: baseAt(deferTok), Call: call}
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
