package bytecode

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
)

const (
	spreadGlobal     = "__tbl_spread"
	pushGlobal       = "__tbl_push"
	restArrayGlobal  = "__rest_array"
	restRecordGlobal = "__rest_record"
)

func hasOptionalLink(e ast.Expression) bool {
	for {
		switch n := e.(type) {
		case *ast.IndexExpression:
			if n.Optional {
				return true
			}
			e = n.Object
		case *ast.MethodCallExpression:
			if n.Optional {
				return true
			}
			e = n.Object
		case *ast.CallExpression:
			e = n.Func
		default:
			return false
		}
	}
}

func (g *Generator) compileOptionalChain(is *InstructionSet, e ast.Expression) {
	end := &anchor{}
	g.compileChainLink(is, e, end)
	end.line = is.count
}

func (g *Generator) compileChainLink(is *InstructionSet, e ast.Expression, end *anchor) {
	switch n := e.(type) {
	case *ast.IndexExpression:
		g.compileChainSpine(is, n.Object, end)
		if n.Optional {
			g.emitNilGuard(is, n.Line(), end)
		}
		if n.IsDot {
			if s, ok := n.Index.(*ast.StringLiteral); ok {
				is.define(GetField, n.Line(), s.Value)
				return
			}
		}
		g.compileExpression(is, n.Index)
		is.define(GetTable, n.Line())

	case *ast.MethodCallExpression:
		g.compileChainSpine(is, n.Object, end)
		if n.Optional {
			g.emitNilGuard(is, n.Line(), end)
		}
		variadic := len(n.Args) > 0 && isMultiValue(n.Args[len(n.Args)-1])
		if variadic {
			is.define(MarkArgs, n.Line())
		}
		is.define(Self, n.Line(), n.Method)
		g.compileCallArgs(is, n.Args, n.Line())
		if variadic {
			is.define(Call, n.Line(), -1, 1)
			return
		}
		is.define(Call, n.Line(), len(n.Args)+1, 1)

	case *ast.CallExpression:
		g.compileChainSpine(is, n.Func, end)
		variadic := len(n.Args) > 0 && isMultiValue(n.Args[len(n.Args)-1])
		if variadic {
			is.define(MarkArgs, n.Line())
		}
		g.compileCallArgs(is, n.Args, n.Line())
		nargs := len(n.Args)
		if variadic {
			nargs = -1
		}
		is.define(Call, n.Line(), nargs, 1)

	default:
		g.compileExpression(is, e)
	}
}

func (g *Generator) compileChainSpine(is *InstructionSet, e ast.Expression, end *anchor) {
	switch e.(type) {
	case *ast.IndexExpression, *ast.MethodCallExpression, *ast.CallExpression:
		g.compileChainLink(is, e, end)
	default:
		g.compileExpression(is, e)
	}
}

func (g *Generator) emitNilGuard(is *InstructionSet, line int, end *anchor) {
	jn := is.define(JumpIfNil, line, end)
	g.current.recordPending(jn)
}

func (g *Generator) compileCoalesce(is *InstructionSet, e *ast.BinaryExpression) {
	g.compileExpression(is, e.Left)
	useRight := &anchor{}
	jn := is.define(JumpIfNil, e.Line(), useRight)
	g.current.recordPending(jn)
	end := &anchor{}
	j := is.define(Jump, e.Line(), end)
	g.current.recordPending(j)
	useRight.line = is.count
	is.define(Pop, e.Line(), 1)
	g.compileExpression(is, e.Right)
	end.line = is.count
}

func tableHasSpread(t *ast.TableConstructor) bool {
	for _, f := range t.Fields {
		if f.IsSpread {
			return true
		}
	}
	return false
}

func (g *Generator) compileSpreadTable(is *InstructionSet, t *ast.TableConstructor) {
	line := t.Line()
	is.define(NewTable, line, len(t.Fields), 0)
	slot := g.current.locals.define("(table spread)")
	is.define(SetLocal, line, slot)
	// The next array index lives in its own scratch local rather than being
	// re-derived from the table's length, which a nil element would corrupt.
	cursor := g.current.locals.define("(spread cursor)")
	is.define(LoadInt, line, int64(1))
	is.define(SetLocal, line, cursor)

	lastIdx := len(t.Fields) - 1
	for i, f := range t.Fields {
		switch {
		case f.IsSpread:
			g.emitMergeCall(is, line, spreadGlobal, slot, cursor, f.Value, false)
		case f.Key == nil:
			// A trailing call or `...` expands to all its values, as in a
			// plain constructor.
			g.emitMergeCall(is, line, pushGlobal, slot, cursor, f.Value, i == lastIdx && isMultiValue(f.Value))
		case f.IsBracketed:
			is.define(GetLocal, line, slot)
			g.compileExpression(is, f.Key)
			g.compileExpression(is, f.Value)
			is.define(SetTable, line)
		default:
			ident, ok := f.Key.(*ast.Identifier)
			if !ok {
				panic("table record key must be *ast.Identifier")
			}
			is.define(GetLocal, line, slot)
			g.compileExpression(is, f.Value)
			is.define(SetField, line, ident.Name)
		}
	}
	is.define(GetLocal, line, slot)
}

// emitMergeCall emits `cursor = global(table, cursor, value)`; with multi set,
// every value the expression produces is passed along.
func (g *Generator) emitMergeCall(is *InstructionSet, line int, global string, slot, cursor int, value ast.Expression, multi bool) {
	is.define(GetGlobal, line, global)
	if multi {
		is.define(MarkArgs, line)
	}
	is.define(GetLocal, line, slot)
	is.define(GetLocal, line, cursor)
	if multi {
		g.compileExpressionMulti(is, value, -1)
		is.define(Call, line, -1, 1)
	} else {
		g.compileExpression(is, value)
		is.define(Call, line, 3, 1)
	}
	is.define(SetLocal, line, cursor)
}

func (g *Generator) compileLocalDestructure(is *InstructionSet, s *ast.LocalDestructureStatement) {
	line := s.Line()
	g.emitValue(is, s.Value)
	srcSlot := g.current.locals.define("(destructure src)")
	is.define(SetLocal, line, srcSlot)

	for _, b := range s.Binds {
		switch {
		case b.Rest && s.IsArray:
			is.define(GetGlobal, line, restArrayGlobal)
			is.define(GetLocal, line, srcSlot)
			is.define(LoadInt, line, int64(b.Index))
			is.define(Call, line, 2, 1)
		case b.Rest:
			is.define(GetGlobal, line, restRecordGlobal)
			is.define(GetLocal, line, srcSlot)
			g.emitKeyList(is, line, s.Binds)
			is.define(Call, line, 2, 1)
		case s.IsArray:
			is.define(GetLocal, line, srcSlot)
			is.define(LoadInt, line, int64(b.Index))
			is.define(GetTable, line)
		default:
			is.define(GetLocal, line, srcSlot)
			is.define(GetField, line, b.Key)
		}

		if b.Default != nil {
			g.emitNilDefault(is, line, b.Default)
		}

		if g.isReplTopLevel() {
			is.define(SetGlobal, line, b.Bind)
			continue
		}
		is.define(SetLocal, line, g.current.locals.define(b.Bind))
	}
}

func (g *Generator) emitKeyList(is *InstructionSet, line int, binds []ast.DestructureBind) {
	is.define(NewTable, line, len(binds), 0)
	idx := int64(0)
	for _, b := range binds {
		if b.Rest {
			continue
		}
		idx++
		is.define(Dup, line)
		is.define(LoadInt, line, idx)
		is.define(LoadString, line, b.Key)
		is.define(SetTable, line)
	}
}

func (g *Generator) emitNilDefault(is *InstructionSet, line int, def ast.Expression) {
	useDefault := &anchor{}
	jn := is.define(JumpIfNil, line, useDefault)
	g.current.recordPending(jn)
	end := &anchor{}
	j := is.define(Jump, line, end)
	g.current.recordPending(j)
	useDefault.line = is.count
	is.define(Pop, line, 1)
	g.compileExpression(is, def)
	end.line = is.count
}

func (g *Generator) compileImplStatement(is *InstructionSet, s *ast.ImplStatement) {
	if s.Target == nil {
		return
	}
	for _, m := range s.Members {
		g.compileLoadName(is, s.Target.Name, s.Line())
		g.compileNamedFunction(is, m.Func, s.Target.Name+"."+m.Name)
		is.define(SetField, s.Line(), m.Name)
	}
}
