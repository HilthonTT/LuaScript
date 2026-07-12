package bytecode

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// compileExpression emits code that leaves exactly one value on top of the
// stack. Multi-value producers (call, method-call, vararg) are clamped to a
// single result here. Use compileExpressionMulti for last-position contexts
// that want the full multi-value expansion.
func (g *Generator) compileExpression(is *InstructionSet, exp ast.Expression) {
	if exp == nil {
		return
	}
	line := exp.Line()
	switch e := exp.(type) {
	case *ast.NilLiteral:
		is.define(LoadNil, line, 1)
	case *ast.BooleanLiteral:
		if e.Value {
			is.define(LoadTrue, line)
		} else {
			is.define(LoadFalse, line)
		}
	case *ast.IntegerLiteral:
		is.define(LoadInt, line, e.Value)
	case *ast.FloatLiteral:
		is.define(LoadFloat, line, e.Value)
	case *ast.StringLiteral:
		is.define(LoadString, line, e.Value)
	case *ast.VarargExpression:
		is.define(LoadVararg, line, 1)
	case *ast.Identifier:
		g.compileLoadName(is, e.Name, line)
	case *ast.FunctionExpression:
		g.compileFunctionExpression(is, e)
	case *ast.TableConstructor:
		g.compileTableConstructor(is, e)
	case *ast.IndexExpression:
		g.compileIndexLoad(is, e)
	case *ast.CallExpression:
		g.compileCall(is, e, 1)
	case *ast.MethodCallExpression:
		g.compileMethodCall(is, e, 1)
	case *ast.BinaryExpression:
		g.compileBinary(is, e)
	case *ast.UnaryExpression:
		g.compileUnary(is, e)
	case *ast.IfExpression:
		g.compileIfExpression(is, e)
	case *ast.ParenExpression:
		// `(...)` adjusts a multi-value to exactly one — clamp by routing
		// through compileExpression (which already clamps).
		g.compileExpression(is, e.Inner)
	case *ast.TypeAssertionExpression:
		// Types are erased — `expr :: T` evaluates to whatever expr does.
		g.compileExpression(is, e.Expr)
	default:
		panic(fmt.Sprintf("bytecode: unsupported expression %T", exp))
	}
}

// compileExpressionMulti emits an expression in a context that can absorb
// multiple results. nresults==-1 means "all results"; otherwise that exact
// number must end up on the stack.
func (g *Generator) compileExpressionMulti(is *InstructionSet, exp ast.Expression, nresults int) {
	if exp == nil {
		return
	}
	switch e := exp.(type) {
	case *ast.CallExpression:
		g.compileCall(is, e, nresults)
	case *ast.MethodCallExpression:
		g.compileMethodCall(is, e, nresults)
	case *ast.VarargExpression:
		is.define(LoadVararg, e.Line(), nresults)
	default:
		// Single-value expressions just push one; pad with nils if more
		// results are demanded, or accept the single value if any count.
		g.compileExpression(is, exp)
		if nresults > 1 {
			is.define(LoadNil, exp.Line(), nresults-1)
		}
	}
}

// compileLoadName resolves `name` and emits the appropriate Get* instruction.
func (g *Generator) compileLoadName(is *InstructionSet, name string, line int) {
	if slot, ok := g.current.locals.lookupLocal(name); ok {
		is.define(GetLocal, line, slot)
		return
	}
	if idx, ok := g.resolveUpvalue(g.current, name); ok {
		is.define(GetUpvalue, line, idx)
		return
	}
	is.define(GetGlobal, line, name)
}

// compileStoreName emits the Set* instruction for an identifier target. The
// value to store is expected to be on top of the stack.
func (g *Generator) compileStoreName(is *InstructionSet, name string, line int) {
	if slot, ok := g.current.locals.lookupLocal(name); ok {
		is.define(SetLocal, line, slot)
		return
	}
	if idx, ok := g.resolveUpvalue(g.current, name); ok {
		is.define(SetUpvalue, line, idx)
		return
	}
	is.define(SetGlobal, line, name)
}

// resolveUpvalue walks ancestor function contexts looking for `name`. When
// found, it registers a passthrough upvalue chain and returns the index into
// ctx's upvalue table. The boolean reports whether resolution succeeded.
func (g *Generator) resolveUpvalue(ctx *funcCtx, name string) (int, bool) {
	if ctx.parent == nil {
		return 0, false
	}
	if slot, ok := ctx.parent.locals.lookupLocal(name); ok {
		return addUpvalue(ctx, name, true, slot), true
	}
	if parentIdx, ok := g.resolveUpvalue(ctx.parent, name); ok {
		return addUpvalue(ctx, name, false, parentIdx), true
	}
	return 0, false
}

func addUpvalue(ctx *funcCtx, name string, inStack bool, index int) int {
	for i, u := range ctx.upvals {
		if u.Name == name && u.InStack == inStack && u.Index == index {
			return i
		}
	}
	ctx.upvals = append(ctx.upvals, UpvalueDesc{Name: name, InStack: inStack, Index: index})
	return len(ctx.upvals) - 1
}

func (g *Generator) compileIndexLoad(is *InstructionSet, e *ast.IndexExpression) {
	g.compileExpression(is, e.Object)
	if e.IsDot {
		if s, ok := e.Index.(*ast.StringLiteral); ok {
			is.define(GetField, e.Line(), s.Value)
			return
		}
	}
	g.compileExpression(is, e.Index)
	is.define(GetTable, e.Line())
}

func (g *Generator) compileCall(is *InstructionSet, e *ast.CallExpression, nresults int) {
	g.compileExpression(is, e.Func)
	// When the last argument is multi-value (call/methodcall/vararg) we
	// don't know its actual width until runtime, so we mark the args
	// base on the VM's mark stack and emit Call with nargs=-1; doCall
	// then computes nargs from `top - mark`. Pure-fixed-arity calls keep
	// the fast static path with no mark overhead.
	variadic := len(e.Args) > 0 && isMultiValue(e.Args[len(e.Args)-1])
	if variadic {
		is.define(MarkArgs, e.Line())
	}
	g.compileCallArgs(is, e.Args, e.Line())
	nargs := len(e.Args)
	if variadic {
		nargs = -1
	}
	is.define(Call, e.Line(), nargs, nresults)
}

func (g *Generator) compileMethodCall(is *InstructionSet, e *ast.MethodCallExpression, nresults int) {
	g.compileExpression(is, e.Object) // [..., obj]
	variadic := len(e.Args) > 0 && isMultiValue(e.Args[len(e.Args)-1])
	if variadic {
		// Mark goes BEFORE Self: Self consumes obj and pushes [method, obj]
		// at the same height boundary, so mark = post-MarkArgs height
		// naturally coincides with obj's slot after Self runs — i.e. the
		// args-base position, matching doCall's `argsStart = mark` rule.
		is.define(MarkArgs, e.Line())
	}
	is.define(Self, e.Line(), e.Method) // [..., method, obj]
	g.compileCallArgs(is, e.Args, e.Line())
	if variadic {
		is.define(Call, e.Line(), -1, nresults)
		return
	}
	is.define(Call, e.Line(), len(e.Args)+1, nresults)
}

// compileCallArgs emits each argument. The last argument, if it is a
// multi-value producer (call, methodcall, vararg), is expanded so the call
// receives every result.
func (g *Generator) compileCallArgs(is *InstructionSet, args []ast.Expression, _ int) {
	for i, a := range args {
		if i == len(args)-1 && isMultiValue(a) {
			g.compileExpressionMulti(is, a, -1)
			return
		}
		g.compileExpression(is, a)
	}
}

func isMultiValue(e ast.Expression) bool {
	switch e.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression, *ast.VarargExpression:
		return true
	}
	return false
}

// compileIfExpression emits a branch chain where every arm pushes exactly
// one value, so the whole expression has a net stack effect of +1 — the
// expression analogue of compileIf.
func (g *Generator) compileIfExpression(is *InstructionSet, e *ast.IfExpression) {
	endAnchor := &anchor{}
	for _, c := range e.Clauses {
		g.compileExpression(is, c.Condition)
		nextAnchor := &anchor{}
		jf := is.define(JumpIfFalse, e.Line(), nextAnchor)
		g.current.recordPending(jf)
		g.compileExpression(is, c.Value)
		j := is.define(Jump, e.Line(), endAnchor)
		g.current.recordPending(j)
		nextAnchor.line = is.count
	}
	g.compileExpression(is, e.Else)
	endAnchor.line = is.count
}

func (g *Generator) compileFunctionExpression(is *InstructionSet, e *ast.FunctionExpression) {
	parent := g.pushFunction(fmt.Sprintf("anon@%d", e.Line()), e.Params, e.IsVararg, e.Line())
	g.compileParamDefaults(g.current.is, e.Params)
	if e.Body != nil {
		g.compileBlock(g.current.is, e.Body)
	}
	idx := g.popFunction(parent, e.Line())
	is.define(Closure, e.Line(), idx)
}

// compileParamDefaults emits the function prologue that applies `= default`
// parameter values: for each defaulted parameter p in slot s,
//
//	if p == nil then p = <default> end
//
// Only nil (absent argument or explicit nil) triggers the default — false
// does not, unlike the `p = p or d` idiom. Defaults are applied left to
// right, so a default may reference earlier parameters.
func (g *Generator) compileParamDefaults(is *InstructionSet, params []ast.TypedParam) {
	for slot, p := range params {
		if p.Default == nil {
			continue
		}
		line := p.Default.Line()
		is.define(GetLocal, line, slot)
		is.define(LoadNil, line, 1)
		is.define(Eq, line)
		skip := &anchor{}
		jf := is.define(JumpIfFalse, line, skip)
		g.current.recordPending(jf)
		g.compileExpression(is, p.Default)
		is.define(SetLocal, line, slot)
		skip.line = is.count
	}
}

func (g *Generator) compileTableConstructor(is *InstructionSet, t *ast.TableConstructor) {
	arrayHint, hashHint := 0, 0
	for _, f := range t.Fields {
		if f.Key == nil {
			arrayHint++
		} else {
			hashHint++
		}
	}
	is.define(NewTable, t.Line(), arrayHint, hashHint)

	lastIdx := len(t.Fields) - 1
	arrayIdx := 1
	for i, f := range t.Fields {
		// A trailing array-positional call/vararg expands to ALL its values
		// (`{f()}`, `{1, 2, f()}`, `{...}`). Mark the stack, push every result,
		// then bulk-fill the array part via a variadic SetList (count -1). The
		// table is left on top for the next field / the constructor result.
		if i == lastIdx && f.Key == nil && isMultiValue(f.Value) {
			is.define(MarkArgs, t.Line())
			g.compileExpressionMulti(is, f.Value, -1)
			is.define(SetList, t.Line(), -1, arrayIdx-1)
			continue
		}
		// Each field needs the table to remain on top after the field is
		// stored; SetTable/SetField consume the table, so we Dup first.
		is.define(Dup, t.Line())
		switch {
		case f.Key == nil:
			// Array-positional entry.
			is.define(LoadInt, t.Line(), int64(arrayIdx))
			arrayIdx++
			g.compileExpression(is, f.Value)
			is.define(SetTable, t.Line())
		case f.IsBracketed:
			g.compileExpression(is, f.Key)
			g.compileExpression(is, f.Value)
			is.define(SetTable, t.Line())
		default:
			// Record entry: Key is an *Identifier; treat name as field key.
			ident, ok := f.Key.(*ast.Identifier)
			if !ok {
				panic("table record key must be *ast.Identifier")
			}
			g.compileExpression(is, f.Value)
			is.define(SetField, t.Line(), ident.Name)
		}
	}
}

func (g *Generator) compileBinary(is *InstructionSet, e *ast.BinaryExpression) {
	switch e.Op {
	case "and":
		// a and b: keep a if falsy, else evaluate b.
		g.compileExpression(is, e.Left)
		end := &anchor{}
		ji := is.define(JumpIfFalseKeep, e.Line(), end)
		g.current.recordPending(ji)
		g.compileExpression(is, e.Right)
		end.line = is.count
		return
	case "or":
		g.compileExpression(is, e.Left)
		end := &anchor{}
		ji := is.define(JumpIfTrueKeep, e.Line(), end)
		g.current.recordPending(ji)
		g.compileExpression(is, e.Right)
		end.line = is.count
		return
	}

	g.compileExpression(is, e.Left)
	g.compileExpression(is, e.Right)
	op, ok := binaryOpcodes[e.Op]
	if !ok {
		panic(fmt.Sprintf("bytecode: unknown binary operator %q", e.Op))
	}
	if op == Concat {
		// Concat carries an explicit count; pairwise emission uses 2.
		is.define(op, e.Line(), 2)
		return
	}
	is.define(op, e.Line())
}

func (g *Generator) compileUnary(is *InstructionSet, e *ast.UnaryExpression) {
	g.compileExpression(is, e.Operand)
	op, ok := unaryOpcodes[e.Op]
	if !ok {
		panic(fmt.Sprintf("bytecode: unknown unary operator %q", e.Op))
	}
	is.define(op, e.Line())
}

var binaryOpcodes = map[string]uint8{
	"+":  Add,
	"-":  Sub,
	"*":  Mul,
	"/":  Div,
	"//": FloorDiv,
	"%":  Mod,
	"^":  Pow,
	"..": Concat, // emitted as Concat with implied count 2 (handled at exec time)
	"==": Eq,
	"~=": NotEq,
	"<":  Lt,
	"<=": Le,
	">":  Gt,
	">=": Ge,
	"&":  BitAnd,
	"|":  BitOr,
	"~":  BitXor,
	"<<": Shl,
	">>": Shr,
}

var unaryOpcodes = map[string]uint8{
	"-":   Neg,
	"not": Not,
	"#":   Len,
	"~":   BitNot,
}
