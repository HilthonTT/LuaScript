package parser

// `match` is implemented as a pure parser-level desugar, mirroring the
// strategy used for compound assignment (compiler/parser/compound_operators.go):
// no new AST node, no codegen change, no typecheck rule. The rewrite emits
// an existing IfStatement wrapped in a DoStatement that binds the scrutinee
// to a fresh local, so the scrutinee is evaluated exactly once even when
// arms have multiple patterns.
//
// Grammar (statement form):
//
//	match <expr> do
//	  <pat> { , <pat> } -> <stmt>
//	  ...
//	  _ -> <stmt>          -- optional wildcard, must be last
//	end
//
// Each arm body is a single statement; use `do ... end` for multiple.
// `_` is recognised as the wildcard pattern (Identifier with name "_") and
// must be the last arm. The pattern token comment in token.go pre-declares
// `Arrow` as the match-arm separator, so this implementation follows that
// design hint rather than introducing a `case`/`then` form.

import (
	"fmt"

	"github.com/hilthontt/sakura-lang/compiler/ast"
	"github.com/hilthontt/sakura-lang/compiler/parser/errors"
	"github.com/hilthontt/sakura-lang/compiler/token"
)

// matchSyntax is the canonical `match` syntax shown in user-facing error
// hints. Kept in one place so we don't drift if the form changes.
const matchSyntax = "match <expr> do <pat>, ... -> <stmt> ... [_ -> <stmt>] end"

// parseMatchStatement consumes a `match ... end` block and returns the
// equivalent `do; local __match_N = <scrutinee>; if ... end; end`.
func (p *Parser) parseMatchStatement() ast.Statement {
	matchTok := p.curToken
	p.nextToken() // consume 'match'

	// Targeted error if the scrutinee is missing: e.g. `match do ... end`.
	// parseExpression would otherwise emit a generic "unexpected token at
	// start of expression", which doesn't say *why* an expression was
	// expected here.
	if p.curTokenIs(token.Do) || p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
		p.errorAt(p.curToken, errors.SyntaxError, "match",
			"expected an expression to scrutinise after 'match', got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}

	scrutinee := p.parseExpression()
	if scrutinee == nil {
		return nil
	}

	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected 'do' after the scrutinee expression, got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}
	p.nextToken() // consume 'do'

	// Fresh binding name per `match` keeps nested matches unambiguous.
	p.matchCounter++
	name := fmt.Sprintf("__match_%d", p.matchCounter)

	ifStmt := &ast.IfStatement{BaseNode: baseAt(matchTok)}

	for !p.curTokenIs(token.End) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(matchTok, errors.EndOfFileError, "match",
				fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
				"each arm is `<pattern> -> <stmt>`; close the whole construct with `end`")
			return nil
		}

		armTok := p.curToken

		// Pattern list: <expr> { , <expr> }
		first := p.parseExpression()
		if first == nil {
			return nil
		}
		patterns := []ast.Expression{first}
		for p.curTokenIs(token.Comma) {
			p.nextToken() // consume ','
			next := p.parseExpression()
			if next == nil {
				return nil
			}
			patterns = append(patterns, next)
		}

		// Diagnose common arm-separator mistakes BEFORE the generic check:
		//   - `then`  (habit from `if ... then`)
		//   - `=>`    (lexes as `=` followed by `>`)
		//   - `:`     (habit from Rust/Swift-ish match)
		// These give a precise hint instead of a generic "expected ->".
		if !p.curTokenIs(token.Arrow) {
			switch {
			case p.curTokenIs(token.Then):
				p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
					"expected '->' after pattern, got 'then'",
					"`match` uses '->' as the arm separator, not 'then'")
			case p.curTokenIs(token.Assign) && p.peekTokenIs(token.GT):
				p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
					"expected '->' after pattern, got '=>'",
					"`match` uses '->' (Lua arrow) as the arm separator, not '=>'")
			case p.curTokenIs(token.Colon):
				p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
					"expected '->' after pattern, got ':'",
					"`match` uses '->' as the arm separator")
			default:
				p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
					"expected '->' after pattern, got "+describeToken(p.curToken),
					"each arm has the form `<pattern> -> <stmt>`")
			}
			return nil
		}
		p.nextToken() // consume '->'

		// Empty arm body: `1 -> end`. Reject early so the user gets a
		// `match`-specific message instead of a generic parser one.
		if p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"arm body cannot be empty",
				"write one statement after '->', or wrap multiple statements in `do ... end`")
			return nil
		}

		bodyStmt := p.parseStatement()
		if bodyStmt == nil {
			// parseStatement records its own error; bail.
			return nil
		}
		armBody := &ast.Block{
			BaseNode:   &ast.BaseNode{Token: armTok},
			Statements: []ast.Statement{bodyStmt},
		}

		// Allow `;` between arms. Lua chains a string- or table-literal
		// onto the previous prefix-expression even across newlines
		// (e.g. `print("a") "b"` parses as `print("a")("b")`). When the
		// next arm's pattern starts with such a literal, an explicit
		// `;` disambiguates. Multiple semicolons collapse like elsewhere.
		p.skipSemicolons()

		// A single `_` identifier is the wildcard. Anywhere else (e.g. as
		// part of `1, _`) it would just be an ordinary variable reference
		// — the Lua convention — so we only recognise the exact form.
		if isWildcardPattern(patterns) {
			if ifStmt.Else != nil {
				p.errorAt(armTok, errors.SyntaxError, "match",
					"duplicate wildcard '_' arm",
					"`match` allows at most one '_' arm; remove the earlier one or merge them")
				return nil
			}
			ifStmt.Else = armBody
			// Anything after `_` would be unreachable; reject early so the
			// user gets a clear error instead of silent dead code.
			if !p.curTokenIs(token.End) {
				p.errorAt(p.curToken, errors.SyntaxError, "match",
					"wildcard '_' arm must be the last arm",
					"the '_' arm matches anything, so arms following it are unreachable; move '_' to the bottom")
				return nil
			}
			break
		}

		ifStmt.Clauses = append(ifStmt.Clauses, ast.IfClause{
			Condition: buildMatchCondition(name, armTok, patterns),
			Body:      armBody,
		})
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(matchTok, errors.UnexpectedTokenError, "match",
			fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
			"")
		return nil
	}
	p.nextToken() // consume 'end'

	stmts := []ast.Statement{
		buildMatchLocal(name, matchTok, scrutinee),
	}
	// Skip the empty if entirely — preserves single evaluation of the
	// scrutinee for its side effects without emitting dead bytecode.
	if len(ifStmt.Clauses) > 0 || ifStmt.Else != nil {
		stmts = append(stmts, ifStmt)
	}
	return &ast.DoStatement{
		BaseNode: baseAt(matchTok),
		Body: &ast.Block{
			BaseNode:   &ast.BaseNode{Token: matchTok},
			Statements: stmts,
		},
	}
}

// isWildcardPattern reports whether a one-element pattern list is the
// literal `_` identifier.
func isWildcardPattern(patterns []ast.Expression) bool {
	if len(patterns) != 1 {
		return false
	}
	id, ok := patterns[0].(*ast.Identifier)
	return ok && id.Name == "_"
}

// buildMatchLocal builds `local <name> = <value>`.
func buildMatchLocal(name string, tok token.Token, value ast.Expression) *ast.LocalStatement {
	return &ast.LocalStatement{
		BaseNode: baseAt(tok),
		Names:    []ast.LocalName{{Name: name}},
		Values:   []ast.Expression{value},
	}
}

// buildMatchCondition builds `(name == p1) or (name == p2) or ...` for the
// arm's pattern list. With a single pattern this is just one equality.
func buildMatchCondition(name string, tok token.Token, patterns []ast.Expression) ast.Expression {
	var cond ast.Expression
	for _, pat := range patterns {
		eq := &ast.BinaryExpression{
			BaseNode: baseAt(tok),
			Op:       "==",
			Left:     &ast.Identifier{BaseNode: baseAt(tok), Name: name},
			Right:    pat,
		}
		if cond == nil {
			cond = eq
			continue
		}
		cond = &ast.BinaryExpression{
			BaseNode: baseAt(tok),
			Op:       "or",
			Left:     cond,
			Right:    eq,
		}
	}
	return cond
}
