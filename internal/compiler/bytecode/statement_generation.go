package bytecode

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

func (g *Generator) compileStatements(stmts []ast.Statement) {
	for _, s := range stmts {
		g.compileStatement(g.current.is, s)
	}
}

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
		switch e := s.Expression.(type) {
		case *ast.CallExpression:
			g.compileCall(is, e, 0)
		case *ast.MethodCallExpression:
			g.compileMethodCall(is, e, 0)
		default:
			g.compileExpression(is, s.Expression)
			is.define(Pop, s.Line(), 1)
		}
	default:
		panic(fmt.Sprintf("bytecode: unsupported statement %T", stmt))
	}
}

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
	needed := target - pushedSoFar
	switch {
	case needed <= 0:
		g.compileExpression(is, last)
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

	if n == 1 && len(s.Values) == 1 {
		g.compileAssignOne(is, s.Targets[0], s.Values[0], s.Line())
		return
	}

	g.current.locals.openScope()
	tempBase := g.current.locals.maxSlot
	_ = tempBase

	if len(s.Values) == n {
		for i, val := range s.Values {
			g.nameFunc(val, assignTargetName(s.Targets[i]))
		}
	}
	g.emitExplistTo(is, s.Values, n, s.Line())
	tempSlots := make([]int, n)
	for i := 0; i < n; i++ {
		tempSlots[i] = g.current.locals.define("(assign tmp)")
	}
	for i := n - 1; i >= 0; i-- {
		is.define(SetLocal, s.Line(), tempSlots[i])
	}

	type storePlan struct {
		ident   string
		objSlot int
		field   string
		keySlot int
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

	for i := range s.Targets {
		p := plans[i]
		switch {
		case p.ident != "":
			is.define(GetLocal, s.Line(), tempSlots[i])
			g.compileStoreName(is, p.ident, s.Line())
		case p.field != "":
			is.define(GetLocal, s.Line(), p.objSlot)
			is.define(GetLocal, s.Line(), tempSlots[i])
			is.define(SetField, s.Line(), p.field)
		default:
			is.define(GetLocal, s.Line(), p.objSlot)
			is.define(GetLocal, s.Line(), p.keySlot)
			is.define(GetLocal, s.Line(), tempSlots[i])
			is.define(SetTable, s.Line())
		}
	}

	g.current.locals.closeScope()
}

func (g *Generator) compileAssignOne(is *InstructionSet, target, value ast.Expression, line int) {
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

func (g *Generator) emitValue(is *InstructionSet, e ast.Expression) {
	if isMultiValue(e) {
		g.compileExpressionMulti(is, e, 1)
		return
	}
	g.compileExpression(is, e)
}

func (g *Generator) compileLocal(is *InstructionSet, s *ast.LocalStatement) {
	n := len(s.Names)
	if len(s.Values) == n {
		for i, val := range s.Values {
			g.nameFunc(val, s.Names[i].Name)
		}
	}
	g.emitExplistTo(is, s.Values, n, s.Line())

	if g.isReplTopLevel() {
		for i := n - 1; i >= 0; i-- {
			is.define(SetGlobal, s.Line(), s.Names[i].Name)
		}
		return
	}

	slots := make([]int, n)
	for i, ln := range s.Names {
		slots[i] = g.current.locals.define(ln.Name)
		_ = ln.Attrib
	}
	for i := n - 1; i >= 0; i-- {
		is.define(SetLocal, s.Line(), slots[i])
	}
}

func (g *Generator) compileLocalFunction(is *InstructionSet, s *ast.LocalFunctionStatement) {
	if g.isReplTopLevel() {
		g.compileNamedFunction(is, s.Func, s.Name)
		is.define(SetGlobal, s.Line(), s.Name)
		return
	}

	slot := g.current.locals.define(s.Name)
	g.compileNamedFunction(is, s.Func, s.Name)
	is.define(SetLocal, s.Line(), slot)
}

func (g *Generator) isReplTopLevel() bool {
	return g.REPL &&
		g.current.parent == nil &&
		len(g.current.locals.scopes) == 1
}

func (g *Generator) compileFunctionDecl(is *InstructionSet, s *ast.FunctionDeclaration) {
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

	switch {
	case len(s.DottedFields) == 0 && s.MethodName == "":
		g.compileNamedFunction(is, fn, s.Name.Name)
		g.compileStoreName(is, s.Name.Name, s.Line())
	default:
		g.compileLoadName(is, s.Name.Name, s.Line())
		fields := s.DottedFields
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
		g.compileNamedFunction(is, fn, funcDeclName(s))
		is.define(SetField, s.Line(), setKey)
	}
}

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
