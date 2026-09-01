package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

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

	switch first.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression:
		return &ast.ExpressionStatement{BaseNode: baseAt(tok), Expression: first}
	}
	p.errorAt(tok, errors.SyntaxError, "",
		fmt.Sprintf("expression %q is not a valid statement", first.String()),
		"Lua only allows function calls and assignments at statement position; did you mean `local x = ...` or `x = ...`?")
	return nil
}

func (p *Parser) parseCompoundAssignStatement(tok token.Token, target ast.Expression, binOp string) ast.Statement {
	if !isAssignTarget(target) {
		p.errorAt(tok, errors.InvalidAssignmentError, "",
			fmt.Sprintf("invalid compound-assignment target %q", target.String()),
			"the LHS of `op=` must be a name, a field access (t.x), or an index (t[k])")
		return nil
	}
	opTok := p.curToken
	p.nextToken()
	rhs := p.parseExpression()
	if rhs == nil {
		return nil
	}

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
		return mkAssign(target, target)
	}

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
		p.nextToken()
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
	p.nextToken()
	values := p.parseExpressionList()
	return &ast.AssignStatement{
		BaseNode: baseAt(tok),
		Targets:  targets,
		Values:   values,
	}
}

func isAssignTarget(e ast.Expression) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.IndexExpression:
		return true
	}
	return false
}
