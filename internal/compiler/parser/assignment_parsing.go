package parser

// Parsing for expression statements, assignment, and compound assignment.

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

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

// isAssignTarget reports whether an expression is a valid LHS target. Lua
// permits Name, prefixexp[exp], and prefixexp.Name.
func isAssignTarget(e ast.Expression) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.IndexExpression:
		return true
	}
	return false
}
