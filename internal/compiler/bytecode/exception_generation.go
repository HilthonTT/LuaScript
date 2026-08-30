package bytecode

// Codegen for defer, try/catch, throw, and labels.

import (
	"slices"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// compileDefer lowers `defer <call>` by wrapping the call in a zero-arg
// function literal, then emitting Defer to register that closure on the
// current frame. Wrapping reuses the closure machinery, so the deferred call
// captures the enclosing locals it references as upvalues; the VM runs the
// registered closures in LIFO order when the frame unwinds.
//
// Because capture is by upvalue, a deferred call observes each captured
// variable's value at frame-exit time, not at the point the `defer` statement
// ran. This differs from Go, which snapshots the call's arguments eagerly.
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

// compileTryCatch emits `try <body> catch [e] do <handler> end` as a protected
// region in the *enclosing* frame — no closure is involved, so `return`,
// `break`, and `continue` inside the body act on the function/loop that
// encloses the whole try/catch, and the body's locals are ordinary frame slots.
//
//	    Try     -> catch      ; install a handler on this frame
//	    <try body>
//	    EndTry 1              ; body finished cleanly — drop the handler
//	    Jump    -> done
//	catch:                    ; the VM lands here with the error value pushed
//	    SetLocal errSlot      ; (Pop 1 when the handler names no binding)
//	    <catch body>
//	done:
//
// The handler is *popped before* the catch body runs, so an error raised
// inside catch propagates outward instead of looping back into itself.
// Popping happens in the VM (see dispatchToHandler) rather than via an emitted
// EndTry, because the jump to `catch` is made by the unwind, not by the code.
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
		// The unwind jumped here from mid-body, skipping the body's normal
		// scope-exit CloseUpvalues. Close any upvalue still open over the
		// body's slots before the catch binding (or anything after) reuses
		// them — otherwise a closure that captured a body local reads the
		// value later written into the recycled slot.
		is.define(CloseUpvalues, s.Line(), bodyBase)
	}
	catchBase := g.current.locals.nextSlot
	catchProtos := len(is.Protos)
	g.current.locals.openScope()
	if s.CatchVar != nil {
		// The binding is a normal local of the enclosing function: its slot is
		// inside the frame's pre-allocated local region (below the stack height
		// the handler truncates to), so the unwind cannot clobber it.
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

// compileThrow emits `throw <expr>` as an evaluate-then-raise. The raised
// value is an ordinary Lua error — indistinguishable from `error(<expr>)`, so
// `pcall` sees it, `catch` binds it, and an uncaught one reaches the host the
// same way. Any Lua value may be thrown; it propagates verbatim.
func (g *Generator) compileThrow(is *InstructionSet, s *ast.ThrowStatement) {
	g.compileExpression(is, s.Value)
	is.define(Throw, s.Line())
}

func (g *Generator) compileLabel(_ *InstructionSet, s *ast.LabelStatement) {
	// A label is purely a jump target — emits no instruction; record its
	// position and resolve any pending forward gotos that name it.
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
