package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) walkExpressionDiscard(e ast.Expression) {
	if e == nil {
		return
	}
	_ = c.typeOfExpression(e)
}

func (c *checker) typeOfExpression(e ast.Expression) *Type {
	if e == nil {
		return nilT
	}
	switch n := e.(type) {
	case *ast.NilLiteral:
		return nilT
	case *ast.BooleanLiteral:
		return NewBooleanLiteral(n.Value, "")
	case *ast.IntegerLiteral:
		return NewNumberLiteral(float64(n.Value), "")
	case *ast.FloatLiteral:
		return NewNumberLiteral(n.Value, "")
	case *ast.StringLiteral:
		return NewStringLiteral(n.Value, "")
	case *ast.VarargExpression:
		return anyT
	case *ast.Identifier:
		if t, ok := c.env.lookup(n.Name); ok {
			return t
		}
		return anyT
	case *ast.ParenExpression:
		return c.typeOfExpression(n.Inner)
	case *ast.TypeAssertionExpression:
		_ = c.typeOfExpression(n.Expr)
		return c.resolveAST(n.Type)
	case *ast.FunctionExpression:
		shape := c.functionShapeFromExpr(n)
		c.walkFunctionBody(n, shape.Fn)
		return shape
	case *ast.TableConstructor:
		return c.typeOfTableConstructor(n)
	case *ast.IndexExpression:
		return c.typeOfIndex(n)
	case *ast.CallExpression:
		return c.typeOfCall(n)
	case *ast.MethodCallExpression:
		return c.typeOfMethodCall(n)
	case *ast.BinaryExpression:
		return c.typeOfBinary(n)
	case *ast.UnaryExpression:
		return c.typeOfUnary(n)
	case *ast.IfExpression:
		return c.typeOfIfExpression(n)
	}
	return anyT
}

func (c *checker) typeOfIfExpression(e *ast.IfExpression) *Type {
	c.env.push()
	defer c.env.pop()

	arms := make([]*Type, 0, len(e.Clauses)+1)
	for _, cl := range e.Clauses {
		c.walkExpressionDiscard(cl.Condition)

		c.env.push()
		c.applyRefinement(c.refine(cl.Condition, true))
		arms = append(arms, c.typeOfExpression(cl.Value))
		c.env.pop()

		c.applyRefinement(c.refine(cl.Condition, false))
	}
	arms = append(arms, c.typeOfExpression(e.Else))
	return NewUnion(arms...)
}

func (c *checker) typeOfTableConstructor(t *ast.TableConstructor) *Type {
	for _, f := range t.Fields {
		if f.Key != nil {
			c.walkExpressionDiscard(f.Key)
		}
		c.walkExpressionDiscard(f.Value)
	}
	return anyT
}

func (c *checker) typeOfIndex(e *ast.IndexExpression) *Type {
	base := c.typeOfExpression(e.Object)
	if base.Kind == KindAny {
		return anyT
	}
	if base.Kind != KindTable {
		c.errf(e.Line(), "index-non-table",
			"cannot index a value of type %q", base.String())
		return anyT
	}
	if name, ok := staticIndexName(e); ok {
		for _, f := range base.Table.Fields {
			if f.Key == name {
				return f.Type
			}
		}
		if base.Table.Indexer != nil && assignable(stringT, base.Table.Indexer.Key) {
			return base.Table.Indexer.Value
		}
		c.errf(e.Line(), "missing-field",
			"type %q has no field %q", base.String(), name)
		return anyT
	}
	if base.Table.Indexer != nil {
		return base.Table.Indexer.Value
	}
	return anyT
}

func staticIndexName(e *ast.IndexExpression) (string, bool) {
	if id, ok := e.Index.(*ast.StringLiteral); ok {
		return id.Value, true
	}
	if id, ok := e.Index.(*ast.Identifier); ok && e.IsDot {
		return id.Name, true
	}
	return "", false
}

func (c *checker) typeOfCall(call *ast.CallExpression) *Type {
	callee := c.typeOfExpression(call.Func)
	args := make([]*Type, len(call.Args))
	for i, a := range call.Args {
		args[i] = c.typeOfExpression(a)
	}
	c.invalidateCallRefinements()
	fn := callableShape(callee)
	if fn == nil {
		if callee.Kind == KindAny {
			return anyT
		}
		c.errf(call.Line(), "call-non-function",
			"cannot call a value of type %q", callee.String())
		return anyT
	}
	if fn.Struct != nil && c.isNamedStructCall(call) {
		c.checkNamedStructCall(call, fn.Struct)
		return fn.Returns[0]
	}
	c.checkCallArgs(call.Line(), fn, args)
	if t := c.requireModuleType(call); t != nil {
		return t
	}
	if len(fn.Returns) == 0 {
		return anyT
	}
	if len(fn.TypeParams) > 0 {
		return c.instantiateCall(fn, args)[0]
	}
	return fn.Returns[0]
}

func callableShape(t *Type) *FunctionShape {
	if t == nil {
		return nil
	}
	if t.Kind == KindFunction {
		return t.Fn
	}
	if t.Kind == KindUnion {
		for _, m := range t.Union {
			if m.Kind == KindFunction {
				return m.Fn
			}
		}
	}
	return nil
}

func (c *checker) typeOfMethodCall(call *ast.MethodCallExpression) *Type {
	c.walkExpressionDiscard(call.Object)
	for _, a := range call.Args {
		c.walkExpressionDiscard(a)
	}
	c.invalidateCallRefinements()
	return anyT
}

func (c *checker) checkCallArgs(line int, fn *FunctionShape, args []*Type) {
	required := 0
	for i, p := range fn.Params {
		if !assignable(nilT, p) {
			required = i + 1
		}
	}

	switch {
	case fn.IsVararg:
		if len(args) < required {
			c.errf(line, "arity",
				"call passes %d args, function expects at least %d",
				len(args), required)
			return
		}
		bound := min(len(args), len(fn.Params))
		for i := 0; i < bound; i++ {
			if !assignable(args[i], fn.Params[i]) {
				c.errAssign(line, args[i], fn.Params[i])
			}
		}
		if fn.VarargType != nil && len(args) > len(fn.Params) {
			for _, a := range args[len(fn.Params):] {
				if !assignable(a, fn.VarargType) {
					c.errAssign(line, a, fn.VarargType)
				}
			}
		}
	default:
		if len(args) < required {
			c.errf(line, "arity",
				"call passes %d args, function expects at least %d",
				len(args), required)
			return
		}
		if len(args) > len(fn.Params) {
			c.errf(line, "arity",
				"call passes %d args, function expects at most %d",
				len(args), len(fn.Params))
			return
		}
		for i, a := range args {
			if !assignable(a, fn.Params[i]) {
				c.errAssign(line, a, fn.Params[i])
			}
		}
	}
}

func (c *checker) isNamedStructCall(call *ast.CallExpression) bool {
	if len(call.Args) != 1 {
		return false
	}
	_, ok := call.Args[0].(*ast.TableConstructor)
	return ok
}

func (c *checker) checkNamedStructCall(call *ast.CallExpression, sc *StructCtor) {
	lit := call.Args[0].(*ast.TableConstructor)

	declared := make(map[string]*Type, len(sc.Shape.Fields))
	for _, f := range sc.Shape.Fields {
		declared[f.Key] = f.Type
	}

	provided := map[string]bool{}
	for _, f := range lit.Fields {
		id, ok := f.Key.(*ast.Identifier)
		if !ok || f.IsBracketed {
			c.errf(call.Line(), "struct-bad-field",
				"struct %q is constructed with named fields (`%s{ field = value }`)",
				sc.Name, sc.Name)
			continue
		}
		want, known := declared[id.Name]
		if !known {
			c.errf(call.Line(), "struct-unknown-field",
				"struct %q has no field %q", sc.Name, id.Name)
			continue
		}
		provided[id.Name] = true
		got := c.typeOfExpression(f.Value)
		if !assignable(got, want) {
			c.errAssign(call.Line(), got, want)
		}
	}
	for _, f := range sc.Shape.Fields {
		if provided[f.Key] {
			continue
		}
		if !assignable(nilT, f.Type) {
			c.errf(call.Line(), "struct-missing-field",
				"struct %q is missing required field %q", sc.Name, f.Key)
		}
	}
}

func (c *checker) typeOfBinary(e *ast.BinaryExpression) *Type {
	left := c.typeOfExpression(e.Left)

	var right *Type
	switch e.Op {
	case "and":
		c.env.push()
		c.applyRefinement(c.refine(e.Left, true))
		right = c.typeOfExpression(e.Right)
		c.env.pop()
	case "or":
		c.env.push()
		c.applyRefinement(c.refine(e.Left, false))
		right = c.typeOfExpression(e.Right)
		c.env.pop()
	default:
		right = c.typeOfExpression(e.Right)
	}

	switch e.Op {
	case "+", "-", "*", "/", "//", "%", "^", "&", "|", "~", "<<", ">>":
		c.requireNumber(e.Line(), left, right)
		if left.Kind == KindAny || right.Kind == KindAny {
			return anyT
		}
		return numberT
	case "..":
		c.requireStringLike(e.Line(), left)
		c.requireStringLike(e.Line(), right)
		if left.Kind == KindAny || right.Kind == KindAny {
			return anyT
		}
		return stringT
	case "==", "~=":
		return booleanT
	case "<", "<=", ">", ">=":
		if !sameOrderable(left, right) {
			c.errf(e.Line(), "compare-mismatch",
				"cannot compare %q with %q", left.String(), right.String())
		}
		return booleanT
	case "and":
		falsy := keepKinds(left, KindNil, KindBoolean)
		if falsy.Kind == KindNever {
			return right
		}
		return NewUnion(falsy, right)
	case "or":
		truthy := removeKind(left, KindNil)
		if truthy.Kind == KindNever {
			return right
		}
		return NewUnion(truthy, right)
	}
	return anyT
}

func (c *checker) typeOfUnary(e *ast.UnaryExpression) *Type {
	t := c.typeOfExpression(e.Operand)
	switch e.Op {
	case "-", "~":
		if !assignable(t, numberT) {
			c.errAssign(e.Line(), t, numberT)
		}
		if e.Op == "-" && t.Kind == KindLiteral && t.Lit != nil && t.Lit.Base == KindNumber {
			return NewNumberLiteral(-t.Lit.Num, "")
		}
		return numberT
	case "not":
		return booleanT
	case "#":
		if !(assignable(t, stringT) || t.Kind == KindTable || t.Kind == KindAny) {
			c.errf(e.Line(), "length-bad-operand",
				"the # operator expects a string or table, got %q", t.String())
		}
		return numberT
	}
	return anyT
}

func (c *checker) requireNumber(line int, ts ...*Type) {
	for _, t := range ts {
		if !assignable(t, numberT) {
			c.errAssign(line, t, numberT)
		}
	}
}

func (c *checker) requireStringLike(line int, t *Type) {
	if assignable(t, stringT) || assignable(t, numberT) {
		return
	}
	c.errf(line, "concat-bad-operand",
		"the .. operator expects string or number, got %q", t.String())
}

func sameOrderable(a, b *Type) bool {
	if a.Kind == KindAny || b.Kind == KindAny {
		return true
	}
	return (assignable(a, numberT) && assignable(b, numberT)) ||
		(assignable(a, stringT) && assignable(b, stringT))
}
