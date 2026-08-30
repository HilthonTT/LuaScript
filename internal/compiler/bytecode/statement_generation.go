package bytecode

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// compileStatements emits the top-level chunk's statement list. The generator
// has already opened the chunk's root scope; this routine does not open a
// new one.
func (g *Generator) compileStatements(stmts []ast.Statement) {
	for _, s := range stmts {
		g.compileStatement(g.current.is, s)
	}
}

// compileBlock opens a fresh local scope, emits each statement, emits the
// trailing return (if any), and closes the scope.
func (g *Generator) compileBlock(is *InstructionSet, block *ast.Block) {
	if block == nil {
		return
	}
	g.current.locals.openScope()
	for _, s := range block.Statements {
		g.compileStatement(is, s)
	}
	if block.Return != nil {
		g.compileReturn(is, block.Return)
	}
	g.current.locals.closeScope()
}

// compileScopedBlock compiles a block whose exit ends its locals' lifetimes
// mid-function (do-end, if/else branches, try bodies) and emits CloseUpvalues
// over the block's slots when the block both declared locals and created a
// closure — the same proto-count heuristic emitLoopClose uses. Without it, a
// closure that captured a block local keeps an open upvalue into a slot a
// later block reuses, and writes to the *new* variable leak through the old
// capture. A trailing return needs no close (the frame unwind closes
// everything above base).
//
// It returns the scope's base slot and whether the block hit the heuristic,
// so a caller with a second, non-fall-through path into the block's slots
// (try's error path into catch) can emit its own close.
func (g *Generator) compileScopedBlock(is *InstructionSet, block *ast.Block, line int) (base int, captured bool) {
	base = g.current.locals.nextSlot
	if block == nil {
		return base, false
	}
	protosBefore := len(is.Protos)
	g.current.locals.openScope()
	for _, s := range block.Statements {
		g.compileStatement(is, s)
	}
	if block.Return != nil {
		g.compileReturn(is, block.Return)
	}
	declared := g.current.locals.nextSlot > base
	g.current.locals.closeScope()
	captured = declared && len(is.Protos) > protosBefore
	if captured && block.Return == nil {
		is.define(CloseUpvalues, line, base)
	}
	return base, captured
}

func (g *Generator) compileStatement(is *InstructionSet, stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.AssignStatement:
		g.compileAssign(is, s)
	case *ast.LocalStatement:
		g.compileLocal(is, s)
	case *ast.LocalFunctionStatement:
		g.compileLocalFunction(is, s)
	case *ast.FunctionDeclaration:
		g.compileFunctionDecl(is, s)
	case *ast.TypeAliasStatement:
		// Types are erased before codegen — type aliases produce no bytecode.
		// The type checker reads them from the AST in its own pre-pass.
		_ = s
	case *ast.IfStatement:
		g.compileIf(is, s)
	case *ast.WhileStatement:
		g.compileWhile(is, s)
	case *ast.RepeatStatement:
		g.compileRepeat(is, s)
	case *ast.NumericForStatement:
		g.compileNumericFor(is, s)
	case *ast.GenericForStatement:
		g.compileGenericFor(is, s)
	case *ast.DoStatement:
		g.compileScopedBlock(is, s.Body, s.Line())
	case *ast.ReturnStatement:
		g.compileReturn(is, s)
	case *ast.BreakStatement:
		g.compileBreak(is, s)
	case *ast.ContinueStatement:
		g.compileContinue(is, s)
	case *ast.GotoStatement:
		g.compileGoto(is, s)
	case *ast.LabelStatement:
		g.compileLabel(is, s)
	case *ast.EnumStatement:
		g.compileEnumStatement(is, s)
	case *ast.StructStatement:
		g.compileStructStatement(is, s)
	case *ast.DeferStatement:
		g.compileDefer(is, s)
	case *ast.MatchStatement:
		g.compileMatch(is, s)
	case *ast.TryCatchStatement:
		g.compileTryCatch(is, s)
	case *ast.ThrowStatement:
		g.compileThrow(is, s)
	case *ast.ExpressionStatement:
		// Lua only allows function/method calls in this slot. We emit the
		// expression with no expected results and pop any value it leaves.
		switch e := s.Expression.(type) {
		case *ast.CallExpression:
			g.compileCall(is, e, 0)
		case *ast.MethodCallExpression:
			g.compileMethodCall(is, e, 0)
		default:
			// Defensive: still compile and discard so the AST shape isn't
			// fatal at codegen time. The parser is the proper enforcer.
			g.compileExpression(is, s.Expression)
			is.define(Pop, s.Line(), 1)
		}
	default:
		panic(fmt.Sprintf("bytecode: unsupported statement %T", stmt))
	}
}

// emitExplistTo pushes exactly `target` values for the given expression list.
// The last expression, if it is a multi-value producer, is expanded to fill
// remaining slots; otherwise it contributes one value and the rest are nils.
func (g *Generator) emitExplistTo(is *InstructionSet, exprs []ast.Expression, target int, line int) {
	m := len(exprs)
	if m == 0 {
		if target > 0 {
			is.define(LoadNil, line, target)
		}
		return
	}
	for i := 0; i < m-1; i++ {
		g.compileExpression(is, exprs[i])
	}
	last := exprs[m-1]
	pushedSoFar := m - 1
	needed := target - pushedSoFar // slots remaining for the last expression
	switch {
	case needed <= 0:
		g.compileExpression(is, last)
		// Excess values: pop them.
		excess := pushedSoFar + 1 - target
		if excess > 0 {
			is.define(Pop, line, excess)
		}
	case isMultiValue(last):
		g.compileExpressionMulti(is, last, needed)
	default:
		g.compileExpression(is, last)
		if needed > 1 {
			is.define(LoadNil, line, needed-1)
		}
	}
}

func (g *Generator) compileAssign(is *InstructionSet, s *ast.AssignStatement) {
	n := len(s.Targets)

	// `x = e`, `t.f = e` and `t[k] = e` — one target, one value — are the
	// overwhelmingly common shape and need none of the temp-slot staging
	// below. That staging exists so a later target cannot observe an
	// earlier target's store; with a single target there is no such
	// hazard, so the value can be pushed straight into the store opcode.
	if n == 1 && len(s.Values) == 1 {
		g.compileAssignOne(is, s.Targets[0], s.Values[0], s.Line())
		return
	}

	// 1) Evaluate every RHS into N temporary local slots.
	g.current.locals.openScope()
	tempBase := g.current.locals.maxSlot // for documentation only
	_ = tempBase

	// Name function literals after the target they are assigned to, so
	// `M.run = function() end` shows up as 'M.run' in a traceback. Only
	// plain-name and dotted-field targets yield a readable name.
	if len(s.Values) == n {
		for i, val := range s.Values {
			g.nameFunc(val, assignTargetName(s.Targets[i]))
		}
	}
	g.emitExplistTo(is, s.Values, n, s.Line())
	tempSlots := make([]int, n)
	// We have N values on the stack, in left-to-right order with the last
	// value on top. SetLocal pops the top, so allocate slots and store
	// right-to-left.
	for i := 0; i < n; i++ {
		tempSlots[i] = g.current.locals.define("(assign tmp)")
	}
	for i := n - 1; i >= 0; i-- {
		is.define(SetLocal, s.Line(), tempSlots[i])
	}

	// 2) Evaluate every target's object/key sub-expressions into temp slots
	//    BEFORE performing any store. Lua evaluates all expressions before the
	//    assignments run, so a later target's index must not observe an earlier
	//    target's write (`i, t[i] = 2, 10` stores into t[old i], not t[2]).
	type storePlan struct {
		ident   string // non-empty => plain name target
		objSlot int    // temp holding the table (index targets)
		field   string // non-empty => dot field: SetField with this key
		keySlot int    // temp holding the key (bracket index targets)
	}
	plans := make([]storePlan, n)
	for i, t := range s.Targets {
		switch tgt := t.(type) {
		case *ast.Identifier:
			plans[i] = storePlan{ident: tgt.Name}
		case *ast.IndexExpression:
			g.compileExpression(is, tgt.Object)
			objSlot := g.current.locals.define("(assign obj)")
			is.define(SetLocal, s.Line(), objSlot)
			p := storePlan{objSlot: objSlot}
			if tgt.IsDot {
				if sl, ok := tgt.Index.(*ast.StringLiteral); ok {
					p.field = sl.Value
				}
			}
			if p.field == "" {
				g.compileExpression(is, tgt.Index)
				p.keySlot = g.current.locals.define("(assign key)")
				is.define(SetLocal, s.Line(), p.keySlot)
			}
			plans[i] = p
		default:
			panic(fmt.Sprintf("bytecode: invalid assignment target %T", t))
		}
	}

	// 3) Perform the stores now that every sub-expression has been evaluated,
	//    fetching table/key/value from their temp slots.
	for i := range s.Targets {
		p := plans[i]
		switch {
		case p.ident != "":
			is.define(GetLocal, s.Line(), tempSlots[i])
			g.compileStoreName(is, p.ident, s.Line())
		case p.field != "":
			is.define(GetLocal, s.Line(), p.objSlot)    // table
			is.define(GetLocal, s.Line(), tempSlots[i]) // value
			is.define(SetField, s.Line(), p.field)
		default:
			is.define(GetLocal, s.Line(), p.objSlot)    // table
			is.define(GetLocal, s.Line(), p.keySlot)    // key
			is.define(GetLocal, s.Line(), tempSlots[i]) // value
			is.define(SetTable, s.Line())
		}
	}

	g.current.locals.closeScope()
}

// compileAssignOne emits the single-target, single-value assignment
// `target = value` without routing the value through a temp slot. See the
// fast-path comment in compileAssign for why that is safe here.
//
// Sub-expressions of an index target are evaluated before the value, which
// is the order PUC Lua uses for a single assignment. §3.3.3 leaves the order
// unspecified for this case; it pins down only that a *multiple* assignment
// evaluates every value before any store, which compileAssign's general path
// still does.
func (g *Generator) compileAssignOne(is *InstructionSet, target, value ast.Expression, line int) {
	// Name a function literal after the target it is bound to, so
	// `M.run = function() end` shows up as 'M.run' in a traceback.
	g.nameFunc(value, assignTargetName(target))

	switch tgt := target.(type) {
	case *ast.Identifier:
		g.emitValue(is, value)
		g.compileStoreName(is, tgt.Name, line)
	case *ast.IndexExpression:
		g.compileExpression(is, tgt.Object)
		if tgt.IsDot {
			if sl, ok := tgt.Index.(*ast.StringLiteral); ok {
				g.emitValue(is, value)
				is.define(SetField, line, sl.Value)
				return
			}
		}
		g.compileExpression(is, tgt.Index)
		g.emitValue(is, value)
		is.define(SetTable, line)
	default:
		panic(fmt.Sprintf("bytecode: invalid assignment target %T", target))
	}
}

// emitValue pushes exactly one value for `e`, clamping a multi-value
// producer to its first result. Equivalent to emitExplistTo with a
// one-expression list and target 1, without the slice.
func (g *Generator) emitValue(is *InstructionSet, e ast.Expression) {
	if isMultiValue(e) {
		g.compileExpressionMulti(is, e, 1)
		return
	}
	g.compileExpression(is, e)
}

func (g *Generator) compileLocal(is *InstructionSet, s *ast.LocalStatement) {
	n := len(s.Names)
	// `local f = function() end` is the same function to a reader as
	// `local function f`, so give the literal the same traceback name.
	// Only the 1:1 explist form can attribute a name unambiguously.
	if len(s.Values) == n {
		for i, val := range s.Values {
			g.nameFunc(val, s.Names[i].Name)
		}
	}
	g.emitExplistTo(is, s.Values, n, s.Line())

	// REPL convenience: at the chunk-root scope of the main chunk, promote
	// `local x = v` to a global assignment so the binding survives across
	// REPL inputs (each line is otherwise its own chunk and local slots
	// die with the frame). Nested scopes (do/if/loops/functions) still get
	// real locals.
	if g.isReplTopLevel() {
		for i := n - 1; i >= 0; i-- {
			is.define(SetGlobal, s.Line(), s.Names[i].Name)
		}
		return
	}

	// Define locals AFTER evaluating the RHS — Lua scoping says the new
	// names are not yet visible inside the initializers.
	slots := make([]int, n)
	for i, ln := range s.Names {
		slots[i] = g.current.locals.define(ln.Name)
		// Attribs (`<const>`, `<close>`) are accepted at parse time but the
		// stack VM has no semantic enforcement for them yet; ignore.
		_ = ln.Attrib
	}
	for i := n - 1; i >= 0; i-- {
		is.define(SetLocal, s.Line(), slots[i])
	}
}

func (g *Generator) compileLocalFunction(is *InstructionSet, s *ast.LocalFunctionStatement) {
	// REPL convenience (see compileLocal): top-level `local function f` in
	// the main chunk becomes a global assignment so the function survives
	// across REPL inputs. Recursion still works because the body resolves
	// `f` via GetGlobal at call time.
	if g.isReplTopLevel() {
		g.compileNamedFunction(is, s.Func, s.Name)
		is.define(SetGlobal, s.Line(), s.Name)
		return
	}

	// Define the local first so the function body can reference itself
	// recursively (matches Lua's `local function f` shorthand semantics).
	slot := g.current.locals.define(s.Name)
	g.compileNamedFunction(is, s.Func, s.Name)
	is.define(SetLocal, s.Line(), slot)
}

// isReplTopLevel reports whether emission is currently inside the chunk-root
// scope of the main chunk under REPL mode. Used to decide whether top-level
// `local` declarations should be promoted to globals so they persist across
// REPL inputs.
func (g *Generator) isReplTopLevel() bool {
	return g.REPL &&
		g.current.parent == nil &&
		len(g.current.locals.scopes) == 1
}

func (g *Generator) compileFunctionDecl(is *InstructionSet, s *ast.FunctionDeclaration) {
	// For colon notation we conventionally splice an implicit `self`
	// parameter at the front. The AST records MethodName but NOT the
	// implicit self, so we synthesize the parameter list here.
	fn := s.Func
	if s.MethodName != "" {
		selfParam := ast.TypedParam{
			Name: &ast.Identifier{BaseNode: ast.BaseNode{Token: s.Func.Token}, Name: "self"},
		}
		fn = &ast.FunctionExpression{
			BaseNode: s.Func.BaseNode,
			Params:   append([]ast.TypedParam{selfParam}, s.Func.Params...),
			IsVararg: s.Func.IsVararg,
			Body:     s.Func.Body,
		}
	}

	// Emit the closure first; we'll route it to the appropriate target.
	switch {
	case len(s.DottedFields) == 0 && s.MethodName == "":
		// Plain `function name() end`: assigns to the global (or local) `name`.
		g.compileNamedFunction(is, fn, s.Name.Name)
		g.compileStoreName(is, s.Name.Name, s.Line())
	default:
		// Walk the funcname path to obtain the target table on the stack.
		g.compileLoadName(is, s.Name.Name, s.Line())
		fields := s.DottedFields
		// All but the last dotted field is a chain of GetField; the last
		// dotted field becomes the SetField key — unless MethodName is set,
		// in which case every dotted field is a GetField and MethodName is
		// the final key.
		var setKey string
		if s.MethodName != "" {
			for _, f := range fields {
				is.define(GetField, s.Line(), f)
			}
			setKey = s.MethodName
		} else {
			for i := 0; i < len(fields)-1; i++ {
				is.define(GetField, s.Line(), fields[i])
			}
			setKey = fields[len(fields)-1]
		}
		// Render the qualified name the way it was written, so a traceback
		// says "in function 'Account:deposit'" rather than just 'deposit'.
		g.compileNamedFunction(is, fn, funcDeclName(s))
		is.define(SetField, s.Line(), setKey)
	}
}

// assignTargetName renders an assignment target as the name a traceback
// should show for a function literal stored into it: `f` for a plain name,
// `M.run` for a dotted field. Bracket-indexed and computed targets have no
// stable name, so they get "" and fall back to "anon@<line>".
func assignTargetName(t ast.Expression) string {
	switch tgt := t.(type) {
	case *ast.Identifier:
		return tgt.Name
	case *ast.IndexExpression:
		if !tgt.IsDot {
			return ""
		}
		sl, ok := tgt.Index.(*ast.StringLiteral)
		if !ok {
			return ""
		}
		obj := assignTargetName(tgt.Object)
		if obj == "" {
			return sl.Value
		}
		return obj + "." + sl.Value
	}
	return ""
}

// funcDeclName rebuilds the dotted/colon path of a `function a.b:c()` header
// for display in tracebacks.
func funcDeclName(s *ast.FunctionDeclaration) string {
	var b strings.Builder
	b.WriteString(s.Name.Name)
	for _, f := range s.DottedFields {
		b.WriteByte('.')
		b.WriteString(f)
	}
	if s.MethodName != "" {
		b.WriteByte(':')
		b.WriteString(s.MethodName)
	}
	return b.String()
}

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

// compileEnumStatement lowers `enum Name V1, V2, ... end` to the
// equivalent of
//
//	local Name = __enum_freeze({V1=1, V2=2, ...}, "Name")
//
// where `__enum_freeze` is the runtime helper installed by
// native/enumrt at VM startup. The helper attaches a __newindex
// metamethod that raises on assignment and locks __metatable so the
// shield can't be removed.
//
// At REPL top-level the binding is promoted to a global (same rule
// `compileLocal` uses) so the name survives across REPL chunks.
//
// The emit sequence mirrors what compileTableConstructor does for a
// record literal:
//
//	GetGlobal "__enum_freeze"        ; push helper
//	NewTable 0 N                     ; push fresh table
//	for each variant i (1..N):
//	    Dup                          ; copy table reference
//	    LoadInt i                    ; push value
//	    SetField "VARIANT"           ; pops (value, table-copy)
//	LoadString "Name"                ; push enum name for the helper's diagnostic
//	Call 2 1                         ; helper(table, name) → frozen table
//	SetLocal slot / SetGlobal name   ; bind result
func (g *Generator) compileEnumStatement(is *InstructionSet, s *ast.EnumStatement) {
	if s.Name == nil || len(s.Variants) == 0 {
		// Parser already errors on these; defensive guard so codegen
		// doesn't panic on a partial AST under recovery.
		return
	}

	if s.IsTagged() {
		g.compileTaggedEnumStatement(is, s)
		return
	}

	line := s.Line()

	// Stack: [fn]
	is.define(GetGlobal, line, "__enum_freeze")

	// Stack: [fn, t]
	is.define(NewTable, line, 0, len(s.Variants))

	for i, v := range s.Variants {
		// Stack: [fn, t, t]
		is.define(Dup, line)
		// Stack: [fn, t, t, value]
		is.define(LoadInt, line, int64(i+1))
		// SetField pops (value, table-copy); back to [fn, t].
		is.define(SetField, line, v.Name)
	}

	// Stack: [fn, t, name]
	is.define(LoadString, line, s.Name.Name)

	// Stack: [frozen-t]
	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}

// compileTaggedEnumStatement lowers a tagged (sum-type) enum
//
//	enum Shape
//	    Circle(number),
//	    Rect(number, number),
//	    Unit,
//	end
//
// to the equivalent of
//
//	local Shape = __enum_adt("Shape", { Circle = 1, Rect = 2, Unit = 0 })
//
// where the second argument maps each variant name to its payload arity.
// `__enum_adt` (installed by native/enumrt) builds a frozen namespace whose
// payload variants become constructor functions and whose nullary variants
// become singleton tagged values. Payload *types* are erased here — the
// checker already validated them; the runtime only needs the arity.
func (g *Generator) compileTaggedEnumStatement(is *InstructionSet, s *ast.EnumStatement) {
	line := s.Line()

	// Stack: [fn]
	is.define(GetGlobal, line, "__enum_adt")
	// Stack: [fn, name]
	is.define(LoadString, line, s.Name.Name)
	// Stack: [fn, name, arities]
	is.define(NewTable, line, 0, len(s.Variants))

	for _, v := range s.Variants {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(len(v.Payload)))
		is.define(SetField, line, v.Name)
	}

	// Stack: [frozen-namespace]
	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}

// compileStructStatement lowers `struct Name { f1: T, f2: T } end` to the
// equivalent of
//
//	local Name = __struct_define("Name", {"f1", "f2"})
//
// where `__struct_define` is the runtime helper installed by
// native/structrt. It returns a constructor closed over the ordered field
// names; the type annotations are erased here (the checker consumed them).
// Binding follows the same local-vs-global rule enums and locals use so the
// name survives across REPL chunks at top level.
//
//	GetGlobal "__struct_define"      ; push factory
//	LoadString "Name"                ; struct name
//	NewTable N 0                     ; field-name array
//	for each field i (1..N):
//	    Dup; LoadInt i; LoadString "field"; SetTable
//	Call 2 1                         ; factory(name, fields) -> constructor
//	SetLocal slot / SetGlobal name   ; bind constructor
func (g *Generator) compileStructStatement(is *InstructionSet, s *ast.StructStatement) {
	if s.Name == nil || len(s.Fields) == 0 {
		// Parser already errors on these; defensive guard against a partial
		// AST under error recovery.
		return
	}

	line := s.Line()

	// Stack: [factory]
	is.define(GetGlobal, line, "__struct_define")
	// Stack: [factory, name]
	is.define(LoadString, line, s.Name.Name)
	// Stack: [factory, name, fields]
	is.define(NewTable, line, len(s.Fields), 0)

	for i, f := range s.Fields {
		is.define(Dup, line)
		is.define(LoadInt, line, int64(i+1))
		is.define(LoadString, line, f.Name)
		is.define(SetTable, line)
	}

	// Stack: [constructor]
	is.define(Call, line, 2, 1)

	if g.isReplTopLevel() {
		is.define(SetGlobal, line, s.Name.Name)
		return
	}

	slot := g.current.locals.define(s.Name.Name)
	is.define(SetLocal, line, slot)
}
