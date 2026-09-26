package bytecode

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

func (g *Generator) compileIf(is *InstructionSet, s *ast.IfStatement) {
	endAnchor := &anchor{}
	for i, c := range s.Clauses {
		g.compileExpression(is, c.Condition)
		nextAnchor := &anchor{}
		jf := is.define(JumpIfFalse, s.Line(), nextAnchor)
		g.current.recordPending(jf)
		g.compileScopedBlock(is, c.Body, s.Line())
		if i < len(s.Clauses)-1 || s.Else != nil {
			j := is.define(Jump, s.Line(), endAnchor)
			g.current.recordPending(j)
		}
		nextAnchor.line = is.count
	}
	if s.Else != nil {
		g.compileScopedBlock(is, s.Else, s.Line())
	}
	endAnchor.line = is.count
}

func (g *Generator) emitLoopClose(is *InstructionSet, baseSlot, protosBefore, line int) {
	if len(is.Protos) > protosBefore {
		is.define(CloseUpvalues, line, baseSlot)
	}
}

func (g *Generator) compileWhile(is *InstructionSet, s *ast.WhileStatement) {
	topAnchor := &anchor{line: is.count}
	g.compileExpression(is, s.Condition)
	exitAnchor := &anchor{}
	jf := is.define(JumpIfFalse, s.Line(), exitAnchor)
	g.current.recordPending(jf)

	closeBase := g.current.locals.nextSlot
	protos := len(is.Protos)
	contAnchor := &anchor{}
	g.pushLoop(&loopFrame{breakAnchor: exitAnchor, continueAnchor: contAnchor})
	g.compileBlock(is, s.Body)
	g.popLoop()
	contAnchor.line = is.count
	g.emitLoopClose(is, closeBase, protos, s.Line())

	jb := is.define(Jump, s.Line(), topAnchor)
	g.current.recordPending(jb)
	exitAnchor.line = is.count
	g.emitLoopClose(is, closeBase, protos, s.Line())
}

func (g *Generator) compileRepeat(is *InstructionSet, s *ast.RepeatStatement) {
	topAnchor := &anchor{line: is.count}
	exitAnchor := &anchor{}

	g.openScope()
	contAnchor := &anchor{}
	frame := &loopFrame{
		breakAnchor: exitAnchor, continueAnchor: contAnchor,
		isRepeat:            true,
		repeatScopeIdx:      len(g.current.locals.scopes) - 1,
		minContinueBindings: -1,
	}
	g.pushLoop(frame)

	closeBase := g.current.locals.nextSlot
	protos := len(is.Protos)
	if s.Body != nil {
		for _, st := range s.Body.Statements {
			g.compileStatement(is, st)
		}
		if s.Body.Return != nil {
			g.compileReturn(is, s.Body.Return)
		}
	}
	contAnchor.line = is.count
	condStart := is.count
	condProtos := len(is.Protos)
	g.compileExpression(is, s.Condition)
	if frame.minContinueBindings >= 0 {
		g.checkRepeatContinueLocals(is, frame, condStart, condProtos, s.Condition.Line())
	}
	g.closeScope(is, s.Line())

	g.popLoop()

	g.emitLoopClose(is, closeBase, protos, s.Line())
	jb := is.define(JumpIfFalse, s.Line(), topAnchor)
	g.current.recordPending(jb)
	exitAnchor.line = is.count
	g.emitLoopClose(is, closeBase, protos, s.Line())
}

func (g *Generator) compileNumericFor(is *InstructionSet, s *ast.NumericForStatement) {
	g.compileExpression(is, s.Start)
	g.compileExpression(is, s.Limit)
	if s.Step != nil {
		g.compileExpression(is, s.Step)
	} else {
		is.define(LoadInt, s.Line(), int64(1))
	}

	g.openScope()
	indexSlot := g.current.locals.define("(for index)")
	g.current.locals.define("(for limit)")
	g.current.locals.define("(for step)")
	varSlot := g.current.locals.define(s.Name)
	is.define(SetLocal, s.Line(), indexSlot+2)
	is.define(SetLocal, s.Line(), indexSlot+1)
	is.define(SetLocal, s.Line(), indexSlot)

	exitAnchor := &anchor{}
	fp := is.define(ForPrep, s.Line(), indexSlot, exitAnchor)
	g.current.recordPending(fp)

	bodyTop := &anchor{line: is.count}
	protos := len(is.Protos)
	contAnchor := &anchor{}
	g.pushLoop(&loopFrame{breakAnchor: exitAnchor, continueAnchor: contAnchor})
	g.compileBlock(is, s.Body)
	g.popLoop()
	contAnchor.line = is.count
	g.emitLoopClose(is, varSlot, protos, s.Line())

	fl := is.define(ForLoop, s.Line(), indexSlot, bodyTop)
	g.current.recordPending(fl)
	exitAnchor.line = is.count
	g.emitLoopClose(is, varSlot, protos, s.Line())

	g.closeScope(is, s.Line())
}

func (g *Generator) compileGenericFor(is *InstructionSet, s *ast.GenericForStatement) {
	g.emitExplistTo(is, s.Exprs, 3, s.Line())

	g.openScope()
	hiddenBase := g.current.locals.define("(for iter)")
	g.current.locals.define("(for state)")
	g.current.locals.define("(for control)")
	is.define(SetLocal, s.Line(), hiddenBase+2)
	is.define(SetLocal, s.Line(), hiddenBase+1)
	is.define(SetLocal, s.Line(), hiddenBase)

	firstVarSlot := g.current.locals.nextSlot
	for _, n := range s.Names {
		g.current.locals.define(n)
	}

	exitAnchor := &anchor{}
	tforAnchor := &anchor{}
	jp := is.define(Jump, s.Line(), tforAnchor)
	g.current.recordPending(jp)

	bodyTop := &anchor{line: is.count}
	protos := len(is.Protos)
	contAnchor := &anchor{}
	g.pushLoop(&loopFrame{breakAnchor: exitAnchor, continueAnchor: contAnchor})
	g.compileBlock(is, s.Body)
	g.popLoop()
	contAnchor.line = is.count
	g.emitLoopClose(is, firstVarSlot, protos, s.Line())

	tforAnchor.line = is.count
	tcall := is.define(TForCall, s.Line(), hiddenBase, len(s.Names))
	_ = tcall
	tloop := is.define(TForLoop, s.Line(), hiddenBase, bodyTop)
	g.current.recordPending(tloop)
	exitAnchor.line = is.count
	g.emitLoopClose(is, firstVarSlot, protos, s.Line())

	g.closeScope(is, s.Line())
}

func (g *Generator) compileReturn(is *InstructionSet, s *ast.ReturnStatement) {
	n := len(s.Values)
	if n == 0 {
		is.define(Return, s.Line(), 0)
		return
	}
	for i := 0; i < n-1; i++ {
		g.compileExpression(is, s.Values[i])
	}
	last := s.Values[n-1]
	if isMultiValue(last) {
		g.compileExpressionMulti(is, last, -1)
		is.define(Return, s.Line(), -1)
		return
	}
	g.compileExpression(is, last)
	is.define(Return, s.Line(), n)
}

func (g *Generator) compileBreak(is *InstructionSet, s *ast.BreakStatement) {
	if len(g.current.loops) == 0 {
		return
	}
	frame := g.current.loops[len(g.current.loops)-1]
	if n := g.exitTBCDepth(frame); n > 0 {
		is.define(CloseTBC, s.Line(), n)
	}
	if n := g.exitTryDepth(frame); n > 0 {
		is.define(EndTry, s.Line(), n)
	}
	j := is.define(Jump, s.Line(), frame.breakAnchor)
	g.current.recordPending(j)
}

func (g *Generator) compileContinue(is *InstructionSet, s *ast.ContinueStatement) {
	if len(g.current.loops) == 0 {
		return
	}
	frame := g.current.loops[len(g.current.loops)-1]
	if frame.isRepeat {
		n := len(g.current.locals.scopes[frame.repeatScopeIdx].bindings)
		if frame.minContinueBindings < 0 || n < frame.minContinueBindings {
			frame.minContinueBindings = n
		}
	}
	if n := g.exitTBCDepth(frame); n > 0 {
		is.define(CloseTBC, s.Line(), n)
	}
	if n := g.exitTryDepth(frame); n > 0 {
		is.define(EndTry, s.Line(), n)
	}
	j := is.define(Jump, s.Line(), frame.continueAnchor)
	g.current.recordPending(j)
}

func (g *Generator) checkRepeatContinueLocals(is *InstructionSet, frame *loopFrame, condStart, condProtos, line int) {
	bindings := g.current.locals.scopes[frame.repeatScopeIdx].bindings
	if frame.minContinueBindings >= len(bindings) {
		return
	}
	skipped := make(map[int]string)
	for _, b := range bindings[frame.minContinueBindings:] {
		if !strings.HasPrefix(b.Name, "(") {
			skipped[b.Slot] = b.Name
		}
	}
	report := func(name string) {
		g.errs = append(g.errs, fmt.Errorf(
			"line %d: local '%s' is used in the repeat...until condition but a 'continue' can skip its declaration",
			line, name))
	}
	for _, ins := range is.Instructions[condStart:is.count] {
		if ins.Opcode == GetLocal || ins.Opcode == SetLocal {
			if name, ok := skipped[int(ins.A)]; ok {
				report(name)
				return
			}
		}
	}
	for _, p := range is.Protos[condProtos:] {
		for _, u := range p.Upvalues {
			if u.InStack {
				if name, ok := skipped[u.Index]; ok {
					report(name)
					return
				}
			}
		}
	}
}

// compileGoto leaves every scope between the goto and its label the way a
// block exit would: to-be-closed variables declared since are closed and
// captured locals get their upvalues closed. Without the latter, a closure
// made in an exited scope keeps pointing at a stack slot that a later local
// reuses — and a backward goto would hand every pass the same variable.
func (g *Generator) compileGoto(is *InstructionSet, s *ast.GotoStatement) {
	slot, tbcDepth := g.current.locals.nextSlot, g.current.tbcDepth
	if lbl, ok := g.findLabel(s.Label); ok {
		g.checkGotoTryRegions(s.Label, s.Line(), g.current.tryRegions, lbl.tryRegions)
		if n := tbcDepth - lbl.tbcDepth; n > 0 {
			is.define(CloseTBC, s.Line(), n)
		}
		if slot > lbl.slot && len(is.Protos) > lbl.protos {
			is.define(CloseUpvalues, s.Line(), lbl.slot)
		}
		is.define(Jump, s.Line(), lbl.line)
		return
	}
	p := pendingGoto{
		label:      s.Label,
		line:       s.Line(),
		tryRegions: slices.Clone(g.current.tryRegions),
		slot:       slot,
		tbcDepth:   tbcDepth,
		depth:      len(g.current.labelScopes) - 1,
	}
	if tbcDepth > 0 {
		p.closeTBC = is.define(CloseTBC, s.Line(), 0)
	}
	if slot > 0 {
		p.closeUpv = is.define(CloseUpvalues, s.Line(), slot)
	}
	p.anchor = &anchor{}
	j := is.define(Jump, s.Line(), p.anchor)
	g.current.recordPending(j)
	g.current.pendingGotos = append(g.current.pendingGotos, p)
}

// patchForwardGoto fills in a forward goto's placeholders once its label's
// scope is known. A placeholder with nothing to do is left as a no-op: a
// CloseTBC of 0, or a CloseUpvalues above every local live at the goto.
func patchForwardGoto(p pendingGoto, lbl labelInfo) {
	if p.closeTBC != nil {
		if n := p.tbcDepth - lbl.tbcDepth; n > 0 {
			setIntParam(p.closeTBC, n)
		}
	}
	if p.closeUpv != nil && lbl.slot < p.slot {
		setIntParam(p.closeUpv, lbl.slot)
	}
}

func setIntParam(ins *Instruction, n int) {
	ins.A = int32(n)
	ins.Params = []any{n}
}

func (g *Generator) checkGotoTryRegions(label string, line int, gotoRegions, labelRegions []int) {
	if slices.Equal(gotoRegions, labelRegions) {
		return
	}
	// Labels are block-scoped, so a label inside a try block is invisible
	// from outside it; leaving one is the only way the regions can differ.
	g.errs = append(g.errs, fmt.Errorf(
		"line %d: 'goto %s' jumps out of a 'try' block — use 'break', 'return', or restructure the control flow",
		line, label))
}
