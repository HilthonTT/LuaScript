package bytecode

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

const matchSubjectName = "(match subject)"

func (g *Generator) compileMatch(is *InstructionSet, s *ast.MatchStatement) {
	line := s.Line()
	protosBefore := len(is.Protos)
	base := g.current.locals.nextSlot

	g.current.locals.openScope()

	g.compileExpression(is, s.Subject)
	subjSlot := g.current.locals.define(matchSubjectName)
	is.define(SetLocal, line, subjSlot)

	endAnchor := &anchor{}
	for i := range s.Arms {
		arm := &s.Arms[i]
		armLine := arm.Line()
		nextAnchor := &anchor{}

		g.current.locals.openScope()

		for _, test := range matchTests(arm) {
			g.compileExpression(is, test)
			jf := is.define(JumpIfFalse, armLine, nextAnchor)
			g.current.recordPending(jf)
		}

		for _, bind := range matchBindings(arm) {
			g.compileStatement(is, bind)
		}

		if arm.Guard != nil {
			g.compileExpression(is, arm.Guard)
			jf := is.define(JumpIfFalse, armLine, nextAnchor)
			g.current.recordPending(jf)
		}

		g.compileStatement(is, arm.Body)
		g.current.locals.closeScope()

		if i < len(s.Arms)-1 {
			j := is.define(Jump, armLine, endAnchor)
			g.current.recordPending(j)
		}
		nextAnchor.line = is.count
	}

	endAnchor.line = is.count
	declared := g.current.locals.nextSlot > base
	g.current.locals.closeScope()

	if declared && len(is.Protos) > protosBefore {
		is.define(CloseUpvalues, line, base)
	}
}

func matchTests(arm *ast.MatchStmtArm) []ast.Expression {
	p := &arm.Pattern
	tok := arm.Token

	switch p.Kind {
	case ast.MatchValue:
		var cond ast.Expression
		for _, v := range p.Values {
			eq := &ast.BinaryExpression{
				BaseNode: ast.BaseNode{Token: tok},
				Op:       "==",
				Left:     matchIdent(tok),
				Right:    v,
			}
			if cond == nil {
				cond = eq
			} else {
				cond = &ast.BinaryExpression{
					BaseNode: ast.BaseNode{Token: tok},
					Op:       "or",
					Left:     cond,
					Right:    eq,
				}
			}
		}
		if cond == nil {
			return nil
		}
		return []ast.Expression{cond}

	case ast.MatchWildcard:
		return nil

	case ast.MatchTyped:
		if t := matchTypeTest(tok, p.Type); t != nil {
			return []ast.Expression{t}
		}
		return nil

	case ast.MatchDestructurePos:
		return []ast.Expression{
			eqStr(tok, callGlobal(tok, "type", matchIdent(tok)), "table"),
			eqStr(tok, matchField(tok, "__tag"), p.Tag),
		}

	case ast.MatchDestructureNamed:
		return []ast.Expression{
			eqStr(tok, callGlobal(tok, "typeof", matchIdent(tok)), p.Tag),
		}
	}
	return nil
}

func matchBindings(arm *ast.MatchStmtArm) []ast.Statement {
	p := &arm.Pattern
	tok := arm.Token

	var out []ast.Statement
	switch p.Kind {
	case ast.MatchTyped:
		if p.Bind != "" && p.Bind != "_" {
			out = append(out, matchLocal(tok, p.Bind, matchIdent(tok)))
		}
	case ast.MatchDestructurePos:
		for i, name := range p.PosBinds {
			if name == "_" {
				continue
			}
			out = append(out, matchLocal(tok, name, matchIndex(tok, int64(i+1))))
		}
	case ast.MatchDestructureNamed:
		for _, nb := range p.NamedBinds {
			if nb.Bind == "_" {
				continue
			}
			out = append(out, matchLocal(tok, nb.Bind, matchField(tok, nb.Field)))
		}
	}
	return out
}

func matchTypeTest(tok token.Token, ty ast.TypeNode) ast.Expression {
	switch t := ty.(type) {
	case *ast.TypePrimitive:
		switch t.Name {
		case "any", "unknown":
			return nil
		case "nil":
			return &ast.BinaryExpression{
				BaseNode: ast.BaseNode{Token: tok},
				Op:       "==",
				Left:     matchIdent(tok),
				Right:    &ast.NilLiteral{BaseNode: ast.BaseNode{Token: tok}},
			}
		default:
			return eqStr(tok, callGlobal(tok, "type", matchIdent(tok)), t.Name)
		}
	case *ast.TypeName:
		return eqStr(tok, callGlobal(tok, "typeof", matchIdent(tok)), t.Name)
	}
	return nil
}

func matchIdent(tok token.Token) *ast.Identifier {
	return &ast.Identifier{BaseNode: ast.BaseNode{Token: tok}, Name: matchSubjectName}
}

func eqStr(tok token.Token, left ast.Expression, s string) ast.Expression {
	return &ast.BinaryExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Op:       "==",
		Left:     left,
		Right:    &ast.StringLiteral{BaseNode: ast.BaseNode{Token: tok}, Value: s},
	}
}

func matchField(tok token.Token, name string) *ast.IndexExpression {
	return &ast.IndexExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Object:   matchIdent(tok),
		Index:    &ast.StringLiteral{BaseNode: ast.BaseNode{Token: tok}, Value: name},
		IsDot:    true,
	}
}

func matchIndex(tok token.Token, i int64) *ast.IndexExpression {
	return &ast.IndexExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Object:   matchIdent(tok),
		Index:    &ast.IntegerLiteral{BaseNode: ast.BaseNode{Token: tok}, Value: i},
	}
}

func callGlobal(tok token.Token, fn string, arg ast.Expression) *ast.CallExpression {
	return &ast.CallExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Func:     &ast.Identifier{BaseNode: ast.BaseNode{Token: tok}, Name: fn},
		Args:     []ast.Expression{arg},
	}
}

func matchLocal(tok token.Token, name string, value ast.Expression) *ast.LocalStatement {
	return &ast.LocalStatement{
		BaseNode: ast.BaseNode{Token: tok},
		Names:    []ast.LocalName{{Name: name}},
		Values:   []ast.Expression{value},
	}
}
