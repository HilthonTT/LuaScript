package bytecode

import (
	"slices"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

func (g *Generator) compileDefer(is *InstructionSet, s *ast.DeferStatement) {
	at := s.BaseNode
	wrapper := &ast.FunctionExpression{
		BaseNode: at,
		Body: &ast.Block{
			BaseNode:   at,
			Statements: []ast.Statement{&ast.ExpressionStatement{BaseNode: at, Expression: s.Call}},
		},
	}
	g.compileFunctionExpression(is, wrapper)
	is.define(Defer, s.Line())
}

func (g *Generator) compileTryCatch(is *InstructionSet, s *ast.TryCatchStatement) {
	catchAnchor := &anchor{}
	doneAnchor := &anchor{}

	t := is.define(Try, s.Line(), catchAnchor)
	g.current.recordPending(t)

	g.current.tryDepth++
	g.nextTryRegion++
	g.current.tryRegions = append(g.current.tryRegions, g.nextTryRegion)
	bodyBase, bodyCaptured := g.compileScopedBlock(is, s.Try, s.Line())
	g.current.tryRegions = g.current.tryRegions[:len(g.current.tryRegions)-1]
	g.current.tryDepth--

	is.define(EndTry, s.Line(), 1)
	j := is.define(Jump, s.Line(), doneAnchor)
	g.current.recordPending(j)

	catchAnchor.line = is.count
	if bodyCaptured {
		is.define(CloseUpvalues, s.Line(), bodyBase)
	}
	catchBase := g.current.locals.nextSlot
	catchProtos := len(is.Protos)
	g.current.locals.openScope()
	if s.CatchVar != nil {
		slot := g.current.locals.define(s.CatchVar.Name)
		is.define(SetLocal, s.CatchVar.Line(), slot)
	} else {
		is.define(Pop, s.Line(), 1)
	}
	g.compileBlock(is, s.Catch)
	declared := g.current.locals.nextSlot > catchBase
	g.current.locals.closeScope()
	if declared && len(is.Protos) > catchProtos {
		is.define(CloseUpvalues, s.Line(), catchBase)
	}

	doneAnchor.line = is.count
}

func (g *Generator) compileThrow(is *InstructionSet, s *ast.ThrowStatement) {
	g.compileExpression(is, s.Value)
	is.define(Throw, s.Line())
}

func (g *Generator) compileLabel(_ *InstructionSet, s *ast.LabelStatement) {
	pos := g.current.is.count
	g.current.labels[s.Name] = labelInfo{line: pos, tryRegions: slices.Clone(g.current.tryRegions)}
	keep := g.current.pendingGotos[:0]
	for _, p := range g.current.pendingGotos {
		if p.label == s.Name {
			g.checkGotoTryRegions(p.label, p.line, p.tryRegions, g.current.tryRegions)
			p.anchor.line = pos
			continue
		}
		keep = append(keep, p)
	}
	g.current.pendingGotos = keep
}
