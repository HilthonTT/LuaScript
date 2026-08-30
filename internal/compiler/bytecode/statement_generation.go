package bytecode

import (
	"fmt"
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
