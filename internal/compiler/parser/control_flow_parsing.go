package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

func (p *Parser) parseIfStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

	stmt := &ast.IfStatement{BaseNode: baseAt(tok)}

	cond := p.parseExpression()
	if !p.curTokenIs(token.Then) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "if",
			"expected 'then' after the condition, got "+describeToken(p.curToken),
			"syntax: `if <cond> then <body> [elseif ...] [else ...] end`")
		return nil
	}
	p.nextToken()
	body := p.parseBlock()
	stmt.Clauses = append(stmt.Clauses, ast.IfClause{Condition: cond, Body: body})

	for p.curTokenIs(token.ElseIf) {
		elseifTok := p.curToken
		p.nextToken()
		c := p.parseExpression()
		if !p.curTokenIs(token.Then) {
			p.errorAt(p.curToken, errors.UnexpectedTokenError, "if",
				"expected 'then' after 'elseif' condition, got "+describeToken(p.curToken),
				fmt.Sprintf("the 'elseif' on line %d needs a 'then' before its body", elseifTok.Line))
			return nil
		}
		p.nextToken()
		b := p.parseBlock()
		stmt.Clauses = append(stmt.Clauses, ast.IfClause{Condition: c, Body: b})
	}

	if p.curTokenIs(token.Else) {
		p.nextToken()
		stmt.Else = p.parseBlock()
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "if",
			fmt.Sprintf("missing 'end' to close 'if' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken()
	return stmt
}

func (p *Parser) parseWhileStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

	cond := p.parseExpression()
	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "while",
			"expected 'do' after the condition, got "+describeToken(p.curToken),
			"syntax: `while <cond> do <body> end`")
		return nil
	}
	p.nextToken()
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "while",
			fmt.Sprintf("missing 'end' to close 'while' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken()
	return &ast.WhileStatement{BaseNode: baseAt(tok), Condition: cond, Body: body}
}

func (p *Parser) parseRepeatStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

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
	p.nextToken()
	cond := p.parseExpression()
	return &ast.RepeatStatement{BaseNode: baseAt(tok), Body: body, Condition: cond}
}

func (p *Parser) parseDoStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()
	body := p.parseBlock()
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "do",
			fmt.Sprintf("missing 'end' to close 'do' block started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken()
	return &ast.DoStatement{BaseNode: baseAt(tok), Body: body}
}

func (p *Parser) parseForStatement() ast.Statement {
	tok := p.curToken
	p.nextToken()

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
	p.nextToken()
	start := p.parseExpression()
	if !p.curTokenIs(token.Comma) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected ',' after the start expression in numeric 'for', got "+describeToken(p.curToken),
			"syntax: `for "+name+" = <start>, <stop>[, <step>] do ... end`")
		return nil
	}
	p.nextToken()
	limit := p.parseExpression()
	var step ast.Expression
	if p.curTokenIs(token.Comma) {
		p.nextToken()
		step = p.parseExpression()
	}
	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected 'do' after the numeric 'for' header, got "+describeToken(p.curToken),
			"syntax: `for "+name+" = <start>, <stop>[, <step>] do ... end`")
		return nil
	}
	p.nextToken()
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "for",
			fmt.Sprintf("missing 'end' to close numeric 'for' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken()
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
		p.nextToken()
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
	p.nextToken()
	exprs := p.parseExpressionList()
	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "for",
			"expected 'do' after the generic-for expression, got "+describeToken(p.curToken),
			"syntax: `for k, v in pairs(t) do ... end`")
		return nil
	}
	p.nextToken()
	p.loopDepth++
	body := p.parseBlock()
	p.loopDepth--
	if !p.curTokenIs(token.End) {
		p.errorAt(tok, errors.UnexpectedTokenError, "for",
			fmt.Sprintf("missing 'end' to close generic 'for' started on line %d", tok.Line),
			"")
		return nil
	}
	p.nextToken()
	return &ast.GenericForStatement{
		BaseNode: baseAt(tok),
		Names:    names,
		Exprs:    exprs,
		Body:     body,
	}
}
