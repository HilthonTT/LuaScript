package parser

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/parser/errors"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

const matchSyntax = "match <expr> do <pattern> [if <guard>] -> <stmt> ... [_ -> <stmt>] end"

func (p *Parser) parseMatchStatement() ast.Statement {
	matchTok := p.curToken
	p.nextToken()

	if p.curTokenIs(token.Do) || p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
		p.errorAt(p.curToken, errors.SyntaxError, "match",
			"expected an expression to scrutinise after 'match', got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}

	subject := p.parseExpression()
	if subject == nil {
		return nil
	}

	if !p.curTokenIs(token.Do) {
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected 'do' after the scrutinee expression, got "+describeToken(p.curToken),
			"syntax: "+matchSyntax)
		return nil
	}
	p.nextToken()

	stmt := &ast.MatchStatement{BaseNode: baseAt(matchTok), Subject: subject}
	sawWildcard := false

	for !p.curTokenIs(token.End) {
		if p.curTokenIs(token.EOF) {
			p.errorAt(matchTok, errors.EndOfFileError, "match",
				fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
				"each arm is `<pattern> [if <guard>] -> <stmt>`; close the whole construct with `end`")
			return nil
		}

		armTok := p.curToken

		pattern, ok := p.parseArmPattern()
		if !ok {
			return nil
		}

		var guard ast.Expression
		if p.curTokenIs(token.If) {
			p.nextToken()
			guard = p.parseExpression()
			if guard == nil {
				return nil
			}
		}

		if !p.curTokenIs(token.Arrow) {
			p.reportArrowError()
			return nil
		}
		p.nextToken()

		if p.curTokenIs(token.End) || p.curTokenIs(token.EOF) {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"arm body cannot be empty",
				"write one statement after '->', or wrap multiple statements in `do ... end`")
			return nil
		}

		var body ast.Statement
		if p.curTokenIs(token.Return) {
			body = p.parseReturnStatement()
		} else {
			body = p.parseStatement()
		}
		if body == nil {
			return nil
		}
		p.skipSemicolons()

		isWildcardArm := pattern.Kind == ast.MatchWildcard && guard == nil
		if isWildcardArm {
			if sawWildcard {
				p.errorAt(armTok, errors.SyntaxError, "match",
					"duplicate wildcard '_' arm",
					"`match` allows at most one unguarded '_' arm")
				return nil
			}
			sawWildcard = true
			if !p.curTokenIs(token.End) {
				p.errorAt(p.curToken, errors.SyntaxError, "match",
					"wildcard '_' arm must be the last arm",
					"the '_' arm matches anything, so arms following it are unreachable; move '_' to the bottom")
				return nil
			}
		}

		stmt.Arms = append(stmt.Arms, ast.MatchStmtArm{
			BaseNode: baseAt(armTok),
			Pattern:  pattern,
			Guard:    guard,
			Body:     body,
		})
	}

	if !p.curTokenIs(token.End) {
		p.errorAt(matchTok, errors.UnexpectedTokenError, "match",
			fmt.Sprintf("missing 'end' to close 'match' started on line %d", matchTok.Line),
			"")
		return nil
	}
	p.nextToken()

	return stmt
}

func (p *Parser) parseArmPattern() (ast.MatchPattern, bool) {
	first, ok := p.parseOnePattern()
	if !ok {
		return ast.MatchPattern{}, false
	}

	for p.curTokenIs(token.Comma) {
		if first.Kind != ast.MatchValue {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"only value patterns can be combined with ','",
				"binding, typed, and destructuring patterns must stand alone")
			return ast.MatchPattern{}, false
		}
		p.nextToken()
		next, ok := p.parseOnePattern()
		if !ok {
			return ast.MatchPattern{}, false
		}
		if next.Kind != ast.MatchValue {
			p.errorAt(p.curToken, errors.SyntaxError, "match",
				"only value patterns can be combined with ','",
				"binding, typed, and destructuring patterns must stand alone")
			return ast.MatchPattern{}, false
		}
		first.Values = append(first.Values, next.Values...)
	}
	return first, true
}

func (p *Parser) parseOnePattern() (ast.MatchPattern, bool) {
	if p.curTokenIs(token.Ident) && p.curToken.Literal == "_" {
		if p.peekTokenIs(token.Colon) {
			return p.parseTypedPattern("_")
		}
		p.nextToken()
		return ast.MatchPattern{Kind: ast.MatchWildcard}, true
	}

	if p.curTokenIs(token.Ident) && p.peekTokenIs(token.Colon) {
		name := p.curToken.Literal
		return p.parseTypedPattern(name)
	}

	expr := p.parseExpression()
	if expr == nil {
		return ast.MatchPattern{}, false
	}
	return p.classifyPattern(expr), true
}

func (p *Parser) parseTypedPattern(name string) (ast.MatchPattern, bool) {
	p.nextToken()
	p.nextToken()
	ty := p.parseType()
	if ty == nil {
		return ast.MatchPattern{}, false
	}
	switch ty.(type) {
	case *ast.TypePrimitive, *ast.TypeName:
	default:
		p.errorAt(p.curToken, errors.SyntaxError, "match",
			"a typed pattern must name a primitive or a type/struct/enum name",
			"e.g. `n: number`, `s: string`, `p: Point`; use a guard for structural tests")
		return ast.MatchPattern{}, false
	}
	return ast.MatchPattern{Kind: ast.MatchTyped, Bind: name, Type: ty}, true
}

func (p *Parser) classifyPattern(expr ast.Expression) ast.MatchPattern {
	valuePattern := ast.MatchPattern{Kind: ast.MatchValue, Values: []ast.Expression{expr}}

	call, ok := expr.(*ast.CallExpression)
	if !ok {
		return valuePattern
	}
	seg, ok := dottedTail(call.Func)
	if !ok {
		return valuePattern
	}

	if len(call.Args) == 1 {
		if tc, ok := call.Args[0].(*ast.TableConstructor); ok {
			if binders, ok := namedBinders(tc); ok && p.structNames[seg] {
				return ast.MatchPattern{Kind: ast.MatchDestructureNamed, Tag: seg, NamedBinds: binders}
			}
			return valuePattern
		}
	}

	if !p.enumVariants[seg] {
		return valuePattern
	}

	binders := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		id, ok := a.(*ast.Identifier)
		if !ok {
			return valuePattern
		}
		binders = append(binders, id.Name)
	}
	return ast.MatchPattern{Kind: ast.MatchDestructurePos, Tag: seg, PosBinds: binders}
}

func dottedTail(e ast.Expression) (string, bool) {
	switch n := e.(type) {
	case *ast.Identifier:
		return n.Name, true
	case *ast.IndexExpression:
		if n.IsDot {
			if s, ok := n.Index.(*ast.StringLiteral); ok {
				return s.Value, true
			}
		}
	}
	return "", false
}

func namedBinders(tc *ast.TableConstructor) ([]ast.MatchFieldBind, bool) {
	if len(tc.Fields) == 0 {
		return nil, false
	}
	out := make([]ast.MatchFieldBind, 0, len(tc.Fields))
	for _, f := range tc.Fields {
		if f.IsBracketed || f.Key == nil {
			return nil, false
		}
		key, ok := f.Key.(*ast.Identifier)
		if !ok {
			return nil, false
		}
		val, ok := f.Value.(*ast.Identifier)
		if !ok {
			return nil, false
		}
		out = append(out, ast.MatchFieldBind{Field: key.Name, Bind: val.Name})
	}
	return out, true
}

func (p *Parser) reportArrowError() {
	switch {
	case p.curTokenIs(token.Then):
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected '->' after pattern, got 'then'",
			"`match` uses '->' as the arm separator, not 'then'")
	case p.curTokenIs(token.Assign) && p.peekTokenIs(token.GT):
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected '->' after pattern, got '=>'",
			"`match` uses '->' (Lua arrow) as the arm separator, not '=>'")
	default:
		p.errorAt(p.curToken, errors.UnexpectedTokenError, "match",
			"expected '->' after pattern, got "+describeToken(p.curToken),
			"each arm has the form `<pattern> [if <guard>] -> <stmt>`")
	}
}
