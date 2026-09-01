package bytecode

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

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
		g.compileExpression(is, e.Inner)
	case *ast.TypeAssertionExpression:
		g.compileExpression(is, e.Expr)
	default:
		panic(fmt.Sprintf("bytecode: unsupported expression %T", exp))
	}
}

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
		g.compileExpression(is, exp)
		if nresults > 1 {
			is.define(LoadNil, exp.Line(), nresults-1)
		}
	}
}

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
	g.compileExpression(is, e.Object)
	variadic := len(e.Args) > 0 && isMultiValue(e.Args[len(e.Args)-1])
	if variadic {
		is.define(MarkArgs, e.Line())
	}
	is.define(Self, e.Line(), e.Method)
	g.compileCallArgs(is, e.Args, e.Line())
	if variadic {
		is.define(Call, e.Line(), -1, nresults)
		return
	}
	is.define(Call, e.Line(), len(e.Args)+1, nresults)
}

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
	g.compileNamedFunction(is, e, g.funcNames[e])
}

func (g *Generator) compileNamedFunction(is *InstructionSet, e *ast.FunctionExpression, name string) {
	if name == "" {
		name = fmt.Sprintf("anon@%d", e.Line())
	}
	parent := g.pushFunction(name, e.Params, e.IsVararg, e.Line())
	g.compileParamDefaults(g.current.is, e.Params)
	if e.Body != nil {
		g.compileBlock(g.current.is, e.Body)
	}
	idx := g.popFunction(parent, e.Line())
	is.define(Closure, e.Line(), idx)
}

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
		if i == lastIdx && f.Key == nil && isMultiValue(f.Value) {
			is.define(MarkArgs, t.Line())
			g.compileExpressionMulti(is, f.Value, -1)
			is.define(SetList, t.Line(), -1, arrayIdx-1)
			continue
		}
		is.define(Dup, t.Line())
		switch {
		case f.Key == nil:
			is.define(LoadInt, t.Line(), int64(arrayIdx))
			arrayIdx++
			g.compileExpression(is, f.Value)
			is.define(SetTable, t.Line())
		case f.IsBracketed:
			g.compileExpression(is, f.Key)
			g.compileExpression(is, f.Value)
			is.define(SetTable, t.Line())
		default:
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
	"..": Concat,
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
