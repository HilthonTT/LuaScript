package bytecode

// Codegen for control flow: if, while, repeat, for, break, continue, goto.

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
		// After the branch, jump past every remaining clause and the else.
		// The very last clause + no else may be optimized, but for clarity
		// we always emit the jump-to-end and then resolve it.
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

// emitLoopClose closes upvalues captured over loop-body locals at slot >=
// baseSlot, but only when the body actually created a closure (its proto count
// grew). This gives each iteration its own fresh variables — Lua 5.4 semantics
// — without imposing per-iteration overhead on the common closure-free loop.
// protosBefore is len(is.Protos) captured immediately before compiling the body.
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

	// Loop-body locals start at the CURRENT live slot count, not maxSlot —
	// the high-water mark can sit above them when an earlier sibling scope
	// used more slots, which would leave body captures unclosed.
	closeBase := g.current.locals.nextSlot
	protos := len(is.Protos)
	contAnchor := &anchor{}
	g.pushLoop(&loopFrame{breakAnchor: exitAnchor, continueAnchor: contAnchor})
	g.compileBlock(is, s.Body)
	g.popLoop()
	// `continue` lands here, so a skipped iteration still closes its
	// captured upvalues before re-testing the condition.
	contAnchor.line = is.count
	g.emitLoopClose(is, closeBase, protos, s.Line())

	jb := is.define(Jump, s.Line(), topAnchor)
	g.current.recordPending(jb)
	exitAnchor.line = is.count
	// `break` jumps here without passing the per-iteration close above, so
	// close again on the exit path (idempotent for the normal-exit case).
	g.emitLoopClose(is, closeBase, protos, s.Line())
}

func (g *Generator) compileRepeat(is *InstructionSet, s *ast.RepeatStatement) {
	topAnchor := &anchor{line: is.count}
	exitAnchor := &anchor{}

	// Repeat's `until` condition is evaluated in the scope of locals declared
	// in the body. We open the scope manually so the condition can see them.
	g.current.locals.openScope()
	contAnchor := &anchor{}
	frame := &loopFrame{
		breakAnchor: exitAnchor, continueAnchor: contAnchor,
		isRepeat:            true,
		repeatScopeIdx:      len(g.current.locals.scopes) - 1,
		minContinueBindings: -1,
	}
	g.pushLoop(frame)

	// See compileWhile: use the live slot count, not the high-water mark.
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
	// `continue` jumps straight to the `until` condition.
	contAnchor.line = is.count
	condStart := is.count
	condProtos := len(is.Protos)
	g.compileExpression(is, s.Condition)
	// A local declared after a `continue` is in scope for the condition but
	// its initialization is skipped on continuing iterations, leaving its
	// reused slot holding an internal temporary. Reject conditions that read
	// such a local (directly, or captured by a closure) — same as Luau.
	if frame.minContinueBindings >= 0 {
		g.checkRepeatContinueLocals(is, frame, condStart, condProtos, s.Condition.Line())
	}
	g.current.locals.closeScope()

	g.popLoop()

	g.emitLoopClose(is, closeBase, protos, s.Line())
	// Falsy condition → jump back; truthy → fall through and exit.
	jb := is.define(JumpIfFalse, s.Line(), topAnchor)
	g.current.recordPending(jb)
	exitAnchor.line = is.count
	// `break` jumps here without passing the close above (see compileWhile).
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

	g.current.locals.openScope()
	// Four slots: three hidden control slots followed by the visible variable.
	// The loop counter lives in the hidden slot, so assigning to the visible
	// variable inside the body cannot perturb the iteration (Lua 5.4 §3.3.5:
	// "The loop variable v is local to the loop body"). ForPrep/ForLoop copy
	// the counter into the visible slot at the top of every iteration.
	indexSlot := g.current.locals.define("(for index)")
	g.current.locals.define("(for limit)")
	g.current.locals.define("(for step)")
	varSlot := g.current.locals.define(s.Name)
	is.define(SetLocal, s.Line(), indexSlot+2) // step
	is.define(SetLocal, s.Line(), indexSlot+1) // limit
	is.define(SetLocal, s.Line(), indexSlot)   // start

	exitAnchor := &anchor{}
	// ForPrep stores the starting value and either falls through into the
	// body (loop runs) or jumps past ForLoop to exitAnchor (empty loop).
	fp := is.define(ForPrep, s.Line(), indexSlot, exitAnchor)
	g.current.recordPending(fp)

	bodyTop := &anchor{line: is.count}
	protos := len(is.Protos)
	contAnchor := &anchor{}
	g.pushLoop(&loopFrame{breakAnchor: exitAnchor, continueAnchor: contAnchor})
	g.compileBlock(is, s.Body)
	g.popLoop()
	// `continue` lands just before the per-iteration upvalue close + ForLoop.
	contAnchor.line = is.count
	// Close upvalues over the loop variable (varSlot) and any captured body
	// locals so each iteration captures a fresh `i`. The hidden control slots
	// below it are never captured, so the close starts at varSlot.
	g.emitLoopClose(is, varSlot, protos, s.Line())

	fl := is.define(ForLoop, s.Line(), indexSlot, bodyTop)
	g.current.recordPending(fl)
	exitAnchor.line = is.count
	// `break` jumps here without passing the close above (see compileWhile).
	g.emitLoopClose(is, varSlot, protos, s.Line())

	g.current.locals.closeScope()
}

func (g *Generator) compileGenericFor(is *InstructionSet, s *ast.GenericForStatement) {
	// Three hidden slots (iterator, state, control) followed by the visible
	// variables (k, v, ...).
	g.emitExplistTo(is, s.Exprs, 3, s.Line())

	g.current.locals.openScope()
	hiddenBase := g.current.locals.define("(for iter)")
	g.current.locals.define("(for state)")
	g.current.locals.define("(for control)")
	is.define(SetLocal, s.Line(), hiddenBase+2)
	is.define(SetLocal, s.Line(), hiddenBase+1)
	is.define(SetLocal, s.Line(), hiddenBase)

	// See compileWhile: the visible variables start at the LIVE slot count.
	// maxSlot is a function-wide high-water mark that can sit above them
	// (earlier sibling scopes), which would leave k/v captures unclosed.
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
	// `continue` lands just before the per-iteration upvalue close + TForCall.
	contAnchor.line = is.count
	// Close upvalues over the visible loop variables (and captured body locals)
	// so each iteration's `k, v` are captured independently.
	g.emitLoopClose(is, firstVarSlot, protos, s.Line())

	tforAnchor.line = is.count
	tcall := is.define(TForCall, s.Line(), hiddenBase, len(s.Names))
	_ = tcall
	tloop := is.define(TForLoop, s.Line(), hiddenBase, bodyTop)
	g.current.recordPending(tloop)
	exitAnchor.line = is.count
	// `break` jumps here without passing the close above (see compileWhile).
	g.emitLoopClose(is, firstVarSlot, protos, s.Line())

	g.current.locals.closeScope()
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
		// Expand the last call/vararg fully and signal "all values from
		// (n-1) up to top of stack" with -1.
		g.compileExpressionMulti(is, last, -1)
		is.define(Return, s.Line(), -1)
		return
	}
	g.compileExpression(is, last)
	is.define(Return, s.Line(), n)
}

func (g *Generator) compileBreak(is *InstructionSet, s *ast.BreakStatement) {
	if len(g.current.loops) == 0 {
		// Outside any loop; the parser should normally catch this.
		return
	}
	frame := g.current.loops[len(g.current.loops)-1]
	// A `break` inside a `try` leaves that protected region — drop its
	// handler, or a later error in this frame would land in a catch clause
	// the program has already jumped out of.
	if n := g.exitTryDepth(frame); n > 0 {
		is.define(EndTry, s.Line(), n)
	}
	j := is.define(Jump, s.Line(), frame.breakAnchor)
	g.current.recordPending(j)
}

// compileContinue jumps to the innermost loop's continue anchor — the point
// right after the body where the loop closes per-iteration upvalues and
// re-tests its condition (while/repeat) or advances its control variable
// (numeric/generic for).
func (g *Generator) compileContinue(is *InstructionSet, s *ast.ContinueStatement) {
	if len(g.current.loops) == 0 {
		// Outside any loop; the parser should normally catch this.
		return
	}
	frame := g.current.loops[len(g.current.loops)-1]
	if frame.isRepeat {
		// Record how many body-scope locals exist at this continue site so
		// compileRepeat can reject an `until` condition that reads a local
		// declared after it (whose initialization this jump skips).
		n := len(g.current.locals.scopes[frame.repeatScopeIdx].bindings)
		if frame.minContinueBindings < 0 || n < frame.minContinueBindings {
			frame.minContinueBindings = n
		}
	}
	// As in compileBreak: a `continue` out of a `try` escapes that region.
	if n := g.exitTryDepth(frame); n > 0 {
		is.define(EndTry, s.Line(), n)
	}
	j := is.define(Jump, s.Line(), frame.continueAnchor)
	g.current.recordPending(j)
}

// checkRepeatContinueLocals rejects an `until` condition that uses a body
// local declared after a `continue` in the same repeat loop. Such a local
// is lexically in scope for the condition, but on iterations that continue
// its declaration never ran and its stack slot may hold a leftover internal
// temporary — silently exposing garbage. Luau reports the same situation as
// a compile error.
//
// Detection is over the emitted code rather than the AST: direct reads show
// up as GetLocal/SetLocal on the skipped slots in the condition's
// instruction range [condStart, is.count); a closure in the condition that
// captures one shows up as an in-stack upvalue on a proto created while
// compiling the condition.
func (g *Generator) checkRepeatContinueLocals(is *InstructionSet, frame *loopFrame, condStart, condProtos, line int) {
	bindings := g.current.locals.scopes[frame.repeatScopeIdx].bindings
	if frame.minContinueBindings >= len(bindings) {
		return
	}
	skipped := make(map[int]string)
	for _, b := range bindings[frame.minContinueBindings:] {
		if !strings.HasPrefix(b.Name, "(") { // ignore compiler temporaries
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

func (g *Generator) compileGoto(is *InstructionSet, s *ast.GotoStatement) {
	if lbl, ok := g.current.labels[s.Label]; ok {
		// Backwards goto — known label.
		g.checkGotoTryRegions(s.Label, s.Line(), g.current.tryRegions, lbl.tryRegions)
		is.define(Jump, s.Line(), lbl.line)
		return
	}
	// Forwards goto: we don't have full label-table fixup yet. Record an
	// anchor; statement_generation's Label handler will resolve it when
	// the label appears.
	a := &anchor{}
	j := is.define(Jump, s.Line(), a)
	g.current.recordPending(j)
	g.current.pendingGotos = append(g.current.pendingGotos, pendingGoto{
		label: s.Label, line: s.Line(), anchor: a,
		tryRegions: slices.Clone(g.current.tryRegions),
	})
}

// checkGotoTryRegions rejects a `goto` whose label does not sit inside the
// exact same `try` regions as the jump. Jumping *out* of a protected region
// would leave its handler installed (so a later error would land in a catch
// the program already left); jumping *in* would run the region with no handler
// installed at all; and a jump between two *sibling* regions — same nesting
// depth, different regions — does both at once, which is why identity is
// compared rather than depth. Rather than miscompile any of these, report it —
// the same call checkRepeatContinueLocals makes for its analogous case. Lua's
// own rule that a goto may not jump into the scope of a local is the closest
// analogue.
func (g *Generator) checkGotoTryRegions(label string, line int, gotoRegions, labelRegions []int) {
	if slices.Equal(gotoRegions, labelRegions) {
		return
	}
	what := "jumps out of a 'try' block"
	switch {
	case len(gotoRegions) < len(labelRegions):
		what = "jumps into a 'try' block"
	case len(gotoRegions) == len(labelRegions):
		what = "jumps between two sibling 'try' blocks"
	}
	g.errs = append(g.errs, fmt.Errorf(
		"line %d: 'goto %s' %s — use 'break', 'return', or restructure the control flow",
		line, label, what))
}
