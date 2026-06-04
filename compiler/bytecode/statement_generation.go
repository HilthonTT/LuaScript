package bytecode

import (
	"fmt"

	"github.com/hilthontt/luascript/compiler/ast"
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
		g.compileBlock(is, s.Body)
	case *ast.ReturnStatement:
		g.compileReturn(is, s)
	case *ast.BreakStatement:
		g.compileBreak(is, s)
	case *ast.GotoStatement:
		g.compileGoto(is, s)
	case *ast.LabelStatement:
		g.compileLabel(is, s)
	case *ast.EnumStatement:
		g.compileEnumStatement(is, s)
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

	// 1) Evaluate every RHS into N temporary local slots.
	g.current.locals.openScope()
	tempBase := g.current.locals.maxSlot // for documentation only
	_ = tempBase

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

	// 2) For each target, emit the appropriate store, fetching the value
	//    from its temp slot.
	for i, t := range s.Targets {
		switch tgt := t.(type) {
		case *ast.Identifier:
			is.define(GetLocal, s.Line(), tempSlots[i])
			g.compileStoreName(is, tgt.Name, s.Line())
		case *ast.IndexExpression:
			useField, key := g.compileIndexStorePrep(is, tgt)
			is.define(GetLocal, s.Line(), tempSlots[i])
			if useField {
				is.define(SetField, s.Line(), key)
			} else {
				is.define(SetTable, s.Line())
			}
		default:
			panic(fmt.Sprintf("bytecode: invalid assignment target %T", t))
		}
	}

	g.current.locals.closeScope()
}

func (g *Generator) compileLocal(is *InstructionSet, s *ast.LocalStatement) {
	n := len(s.Names)
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
		g.compileFunctionExpression(is, s.Func)
		is.define(SetGlobal, s.Line(), s.Name)
		return
	}

	// Define the local first so the function body can reference itself
	// recursively (matches Lua's `local function f` shorthand semantics).
	slot := g.current.locals.define(s.Name)
	g.compileFunctionExpression(is, s.Func)
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
			Name: &ast.Identifier{BaseNode: &ast.BaseNode{Token: s.Func.Token}, Name: "self"},
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
		g.compileFunctionExpression(is, fn)
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
		g.compileFunctionExpression(is, fn)
		is.define(SetField, s.Line(), setKey)
	}
}

func (g *Generator) compileIf(is *InstructionSet, s *ast.IfStatement) {
	endAnchor := &anchor{}
	for i, c := range s.Clauses {
		g.compileExpression(is, c.Condition)
		nextAnchor := &anchor{}
		jf := is.define(JumpIfFalse, s.Line(), nextAnchor)
		g.current.recordPending(jf)
		g.compileBlock(is, c.Body)
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
		g.compileBlock(is, s.Else)
	}
	endAnchor.line = is.count
}

func (g *Generator) compileWhile(is *InstructionSet, s *ast.WhileStatement) {
	topAnchor := &anchor{line: is.count}
	g.compileExpression(is, s.Condition)
	exitAnchor := &anchor{}
	jf := is.define(JumpIfFalse, s.Line(), exitAnchor)
	g.current.recordPending(jf)

	g.current.loops = append(g.current.loops, &loopFrame{breakAnchor: exitAnchor})
	g.compileBlock(is, s.Body)
	g.current.loops = g.current.loops[:len(g.current.loops)-1]

	jb := is.define(Jump, s.Line(), topAnchor)
	g.current.recordPending(jb)
	exitAnchor.line = is.count
}

func (g *Generator) compileRepeat(is *InstructionSet, s *ast.RepeatStatement) {
	topAnchor := &anchor{line: is.count}
	exitAnchor := &anchor{}

	g.current.loops = append(g.current.loops, &loopFrame{breakAnchor: exitAnchor})

	// Repeat's `until` condition is evaluated in the scope of locals declared
	// in the body. We open the scope manually so the condition can see them.
	g.current.locals.openScope()
	if s.Body != nil {
		for _, st := range s.Body.Statements {
			g.compileStatement(is, st)
		}
		if s.Body.Return != nil {
			g.compileReturn(is, s.Body.Return)
		}
	}
	g.compileExpression(is, s.Condition)
	g.current.locals.closeScope()

	g.current.loops = g.current.loops[:len(g.current.loops)-1]

	// Falsy condition → jump back; truthy → fall through and exit.
	jb := is.define(JumpIfFalse, s.Line(), topAnchor)
	g.current.recordPending(jb)
	exitAnchor.line = is.count
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
	indexSlot := g.current.locals.define(s.Name)
	g.current.locals.define("(for limit)")
	g.current.locals.define("(for step)")
	is.define(SetLocal, s.Line(), indexSlot+2) // step
	is.define(SetLocal, s.Line(), indexSlot+1) // limit
	is.define(SetLocal, s.Line(), indexSlot)   // start

	forLoopAnchor := &anchor{}
	exitAnchor := &anchor{}
	fp := is.define(ForPrep, s.Line(), indexSlot, forLoopAnchor)
	g.current.recordPending(fp)

	bodyTop := &anchor{line: is.count}
	g.current.loops = append(g.current.loops, &loopFrame{breakAnchor: exitAnchor})
	g.compileBlock(is, s.Body)
	g.current.loops = g.current.loops[:len(g.current.loops)-1]

	forLoopAnchor.line = is.count
	fl := is.define(ForLoop, s.Line(), indexSlot, bodyTop)
	g.current.recordPending(fl)
	exitAnchor.line = is.count

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

	for _, n := range s.Names {
		g.current.locals.define(n)
	}

	exitAnchor := &anchor{}
	tforAnchor := &anchor{}
	jp := is.define(Jump, s.Line(), tforAnchor)
	g.current.recordPending(jp)

	bodyTop := &anchor{line: is.count}
	g.current.loops = append(g.current.loops, &loopFrame{breakAnchor: exitAnchor})
	g.compileBlock(is, s.Body)
	g.current.loops = g.current.loops[:len(g.current.loops)-1]

	tforAnchor.line = is.count
	tcall := is.define(TForCall, s.Line(), hiddenBase, len(s.Names))
	_ = tcall
	tloop := is.define(TForLoop, s.Line(), hiddenBase, bodyTop)
	g.current.recordPending(tloop)
	exitAnchor.line = is.count

	g.current.locals.closeScope()
}

// ---------------------------------------------------------------------------
// Jumps
// ---------------------------------------------------------------------------

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
	j := is.define(Jump, s.Line(), frame.breakAnchor)
	g.current.recordPending(j)
}

func (g *Generator) compileGoto(is *InstructionSet, s *ast.GotoStatement) {
	if line, ok := g.current.labels[s.Label]; ok {
		// Backwards goto — known label.
		is.define(Jump, s.Line(), line)
		return
	}
	// Forwards goto: we don't have full label-table fixup yet. Record an
	// anchor; statement_generation's Label handler will resolve it when
	// the label appears.
	a := &anchor{}
	j := is.define(Jump, s.Line(), a)
	g.current.recordPending(j)
	g.current.pendingGotos = append(g.current.pendingGotos, pendingGoto{label: s.Label, anchor: a})
}

func (g *Generator) compileLabel(_ *InstructionSet, s *ast.LabelStatement) {
	// A label is purely a jump target — emits no instruction; record its
	// position and resolve any pending forward gotos that name it.
	pos := g.current.is.count
	g.current.labels[s.Name] = pos
	keep := g.current.pendingGotos[:0]
	for _, p := range g.current.pendingGotos {
		if p.label == s.Name {
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
