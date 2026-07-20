package bytecode

// Lowering for the first-class `match` statement.
//
// A match compiles to one test-and-branch chain: the scrutinee is evaluated
// once into a hidden local, then each arm tests, binds, optionally checks its
// guard, and runs its body before jumping past the rest.
//
//	<subject>            ; SetLocal (match subject)
//	arm 1: test... ------------> JumpIfFalse arm 2
//	       bindings
//	       guard --------------> JumpIfFalse arm 2
//	       body
//	                            Jump end
//	arm 2: ...
//	end:   CloseUpvalues (only when an arm captured a binder)
//
// A failed test jumps straight to the next arm, so an arm costs only the
// conjuncts it actually reaches — this is what the old `__matched` flag
// desugar bought at the price of re-reading a local before every arm. A
// failing *guard* falls through to the next arm exactly like a failing test,
// which is the behaviour the flag existed to preserve.
//
// The pattern tests and bindings are built as small AST fragments referencing
// the hidden subject local and handed to the ordinary expression/statement
// compiler, rather than being hand-emitted as opcodes. Nested matches are
// unambiguous even though the hidden local always has the same name: each
// match opens its own scope, so `lookupLocal` finds the innermost subject.

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/token"
)

// matchSubjectName is the hidden local holding the evaluated scrutinee. The
// parentheses keep it out of reach of any identifier a user could write.
const matchSubjectName = "(match subject)"

func (g *Generator) compileMatch(is *InstructionSet, s *ast.MatchStatement) {
	line := s.Line()
	protosBefore := len(is.Protos)
	base := g.current.locals.nextSlot

	g.current.locals.openScope()

	// Evaluate the scrutinee exactly once.
	g.compileExpression(is, s.Subject)
	subjSlot := g.current.locals.define(matchSubjectName)
	is.define(SetLocal, line, subjSlot)

	endAnchor := &anchor{}
	for i := range s.Arms {
		arm := &s.Arms[i]
		armLine := arm.Line()
		nextAnchor := &anchor{}

		// Binders live in a scope of their own so they are visible to the
		// guard and the body but not to later arms.
		g.current.locals.openScope()

		for _, test := range matchTests(arm) {
			g.compileExpression(is, test)
			jf := is.define(JumpIfFalse, armLine, nextAnchor)
			g.current.recordPending(jf)
		}

		// Only reached once the pattern matched, so the projections below
		// cannot fault on a value of the wrong shape.
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

		// The last arm falls through to the end anyway.
		if i < len(s.Arms)-1 {
			j := is.define(Jump, armLine, endAnchor)
			g.current.recordPending(j)
		}
		nextAnchor.line = is.count
	}

	endAnchor.line = is.count
	declared := g.current.locals.nextSlot > base
	g.current.locals.closeScope()

	// Same proto-count heuristic compileScopedBlock uses: if an arm body
	// closed over a binder, that upvalue must not stay open into whatever
	// reuses these slots next. Every non-return path converges here, so one
	// close at the end covers them all.
	if declared && len(is.Protos) > protosBefore {
		is.define(CloseUpvalues, line, base)
	}
}

// matchTests returns the boolean tests an arm's pattern contributes, in
// evaluation order. Each is branched on separately. An always-matching
// pattern (a wildcard, or `: any`) contributes none.
func matchTests(arm *ast.MatchStmtArm) []ast.Expression {
	p := &arm.Pattern
	tok := arm.Token

	switch p.Kind {
	case ast.MatchValue:
		// Alternatives OR together into a single test — they cannot be
		// separate branches, since any one of them matching is enough.
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
		// `type(subject) == "table"` guards the `__tag` read that follows.
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

// matchBindings returns the `local <name> = <projection>` statements an
// arm's pattern introduces. Value and wildcard patterns bind nothing.
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

// matchTypeTest builds the runtime type test for a typed pattern. A
// TypePrimitive probes the Lua-level `type()`; a TypeName probes `typeof()`
// (which reports the nominal `__type` of structs and tagged-enum values).
// `any` always matches, so it yields no test at all.
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

// --- small AST builders, all anchored on the arm's token for line info ---

// matchIdent references the hidden scrutinee local.
func matchIdent(tok token.Token) *ast.Identifier {
	return &ast.Identifier{BaseNode: ast.BaseNode{Token: tok}, Name: matchSubjectName}
}

// eqStr builds `left == "s"`.
func eqStr(tok token.Token, left ast.Expression, s string) ast.Expression {
	return &ast.BinaryExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Op:       "==",
		Left:     left,
		Right:    &ast.StringLiteral{BaseNode: ast.BaseNode{Token: tok}, Value: s},
	}
}

// matchField builds `subject.name`.
func matchField(tok token.Token, name string) *ast.IndexExpression {
	return &ast.IndexExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Object:   matchIdent(tok),
		Index:    &ast.StringLiteral{BaseNode: ast.BaseNode{Token: tok}, Value: name},
		IsDot:    true,
	}
}

// matchIndex builds `subject[i]`.
func matchIndex(tok token.Token, i int64) *ast.IndexExpression {
	return &ast.IndexExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Object:   matchIdent(tok),
		Index:    &ast.IntegerLiteral{BaseNode: ast.BaseNode{Token: tok}, Value: i},
	}
}

// callGlobal builds `fn(arg)`. `fn` resolves like any other name, so a
// user-defined `type`/`typeof` shadows the builtin here exactly as it would
// in hand-written source.
func callGlobal(tok token.Token, fn string, arg ast.Expression) *ast.CallExpression {
	return &ast.CallExpression{
		BaseNode: ast.BaseNode{Token: tok},
		Func:     &ast.Identifier{BaseNode: ast.BaseNode{Token: tok}, Name: fn},
		Args:     []ast.Expression{arg},
	}
}

// matchLocal builds `local name = value`.
func matchLocal(tok token.Token, name string, value ast.Expression) *ast.LocalStatement {
	return &ast.LocalStatement{
		BaseNode: ast.BaseNode{Token: tok},
		Names:    []ast.LocalName{{Name: name}},
		Values:   []ast.Expression{value},
	}
}
