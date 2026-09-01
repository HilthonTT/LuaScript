package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) walkBlock(b *ast.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Statements {
		c.walkStatement(s)
	}
	if b.Return != nil {
		c.walkReturn(b.Return)
	}
}

func (c *checker) walkStatement(s ast.Statement) {
	switch n := s.(type) {
	case *ast.LocalStatement:
		c.walkLocalStatement(n)
	case *ast.LocalFunctionStatement:
		c.walkLocalFunctionStatement(n)
	case *ast.FunctionDeclaration:
		c.walkFunctionDeclaration(n)
	case *ast.AssignStatement:
		c.walkAssignStatement(n)
	case *ast.IfStatement:
		c.walkIfStatement(n)
	case *ast.WhileStatement:
		c.walkExpressionDiscard(n.Condition)
		c.env.push()
		c.widenLoopAssigned(n.Body)
		c.applyRefinement(c.refine(n.Condition, true))
		c.walkBlock(n.Body)
		c.env.pop()
	case *ast.RepeatStatement:
		c.env.push()
		c.widenLoopAssigned(n.Body)
		c.walkBlock(n.Body)
		if blockHasDirectContinue(n.Body) {
			c.env.dropRefinedInTop()
		}
		c.walkExpressionDiscard(n.Condition)
		c.env.pop()
	case *ast.NumericForStatement:
		c.walkNumericFor(n)
	case *ast.GenericForStatement:
		c.walkGenericFor(n)
	case *ast.DoStatement:
		c.env.push()
		c.walkBlock(n.Body)
		c.env.pop()
	case *ast.ExpressionStatement:
		c.walkExpressionDiscard(n.Expression)
		if cond, ok := assertCondition(n.Expression); ok && c.builtinInScope("assert") {
			c.applyRefinement(c.refine(cond, true))
		}
	case *ast.DeferStatement:
		c.walkExpressionDiscard(n.Call)
	case *ast.MatchStatement:
		c.walkMatchStatement(n)
	case *ast.TryCatchStatement:
		c.env.push()
		c.walkBlock(n.Try)
		c.env.pop()
		c.env.push()
		if n.CatchVar != nil {
			c.env.define(n.CatchVar.Name, anyT)
		}
		c.walkBlock(n.Catch)
		c.env.pop()
	case *ast.ThrowStatement:
		c.walkExpressionDiscard(n.Value)
	case *ast.ReturnStatement:
		c.walkReturn(n)
	case *ast.TypeAliasStatement, *ast.LabelStatement, *ast.BreakStatement,
		*ast.ContinueStatement, *ast.GotoStatement:
	case *ast.EnumStatement:
		if n.Name == nil {
			break
		}
		c.recordEnum(n)
		if n.IsTagged() {
			c.env.define(n.Name.Name, c.taggedEnumNamespaceType(n))
			break
		}
		c.env.define(n.Name.Name, classicEnumNamespaceType(n))
	case *ast.StructStatement:
		if n.Name != nil {
			c.env.define(n.Name.Name, c.structConstructorType(n))
		}
	}
}

func (c *checker) walkLocalStatement(s *ast.LocalStatement) {
	values := c.expandRHS(s.Values, len(s.Names))
	for i, name := range s.Names {
		var bound *Type
		if name.Type != nil {
			declared := c.resolveAST(name.Type)
			if !assignable(values[i], declared) {
				c.errAssign(s.Line(), values[i], declared)
			}
			bound = declared
		} else {
			bound = widen(values[i])
			if bound.Kind == KindNil {
				bound = anyT
			}
		}
		c.env.define(name.Name, bound)
	}
}

func (c *checker) walkLocalFunctionStatement(s *ast.LocalFunctionStatement) {
	shape := c.functionShapeFromExpr(s.Func)
	c.env.define(s.Name, shape)
	c.walkFunctionBody(s.Func, shape.Fn)
}

func (c *checker) walkFunctionDeclaration(s *ast.FunctionDeclaration) {
	shape := c.functionShapeFromExpr(s.Func)
	if len(s.DottedFields) == 0 && s.MethodName == "" {
		c.env.define(s.Name.Name, shape)
	}
	c.walkFunctionBody(s.Func, shape.Fn)
}

func (c *checker) walkAssignStatement(s *ast.AssignStatement) {
	rhs := c.expandRHS(s.Values, len(s.Targets))
	for i, t := range s.Targets {
		switch tgt := t.(type) {
		case *ast.Identifier:
			declared, ok := c.env.lookupDeclared(tgt.Name)
			if !ok {
				c.env.define(tgt.Name, widen(rhs[i]))
				continue
			}
			if !assignable(rhs[i], declared) {
				c.errAssign(s.Line(), rhs[i], declared)
			}
			c.env.widenRefined(tgt.Name, widen(rhs[i]))
		case *ast.IndexExpression:
			base := c.typeOfExpression(tgt.Object)
			if base.Kind != KindTable && base.Kind != KindAny {
				c.errf(s.Line(), "index-non-table",
					"cannot index a value of type %q", base.String())
			}
		}
	}
}

func (c *checker) walkMatchStatement(s *ast.MatchStatement) {
	subject := c.typeOfExpression(s.Subject)
	c.checkMatchExhaustive(s, subject)

	for i := range s.Arms {
		arm := &s.Arms[i]

		for _, v := range arm.Pattern.Values {
			c.walkExpressionDiscard(v)
		}

		c.env.push()

		bindT := anyT
		if arm.Pattern.Kind == ast.MatchTyped && arm.Pattern.Type != nil {
			if t := c.resolveAST(arm.Pattern.Type); t != nil {
				bindT = t
			}
		}
		for _, name := range arm.Pattern.Binders() {
			c.env.define(name, bindT)
		}

		if arm.Guard != nil {
			c.walkExpressionDiscard(arm.Guard)
			c.applyRefinement(c.refine(arm.Guard, true))
		}
		c.walkStatement(arm.Body)

		c.env.pop()
	}
}

func (c *checker) walkIfStatement(s *ast.IfStatement) {
	c.env.push()

	var persist []refinement
	prefixTerminates := true

	for _, cl := range s.Clauses {
		c.walkExpressionDiscard(cl.Condition)

		thenR := c.refine(cl.Condition, true)
		c.env.push()
		c.applyRefinement(thenR)
		c.walkBlock(cl.Body)
		c.env.pop()

		negR := c.refine(cl.Condition, false)
		c.applyRefinement(negR)

		if prefixTerminates {
			if c.blockTerminates(cl.Body) {
				persist = append(persist, negR)
			} else {
				prefixTerminates = false
			}
		}
	}

	if s.Else != nil {
		c.env.push()
		c.walkBlock(s.Else)
		c.env.pop()
	}

	c.env.pop()
	for _, r := range persist {
		c.applyRefinement(r)
	}
}

func (c *checker) walkNumericFor(s *ast.NumericForStatement) {
	for _, e := range []ast.Expression{s.Start, s.Limit, s.Step} {
		if e == nil {
			continue
		}
		t := c.typeOfExpression(e)
		if !assignable(t, numberT) {
			c.errAssign(e.Line(), t, numberT)
		}
	}
	c.env.push()
	c.widenLoopAssigned(s.Body)
	c.env.define(s.Name, numberT)
	c.walkBlock(s.Body)
	c.env.pop()
}

func (c *checker) walkGenericFor(s *ast.GenericForStatement) {
	for _, e := range s.Exprs {
		c.walkExpressionDiscard(e)
	}
	c.env.push()
	c.widenLoopAssigned(s.Body)
	for _, name := range s.Names {
		c.env.define(name, anyT)
	}
	c.walkBlock(s.Body)
	c.env.pop()
}

func (c *checker) walkReturn(r *ast.ReturnStatement) {
	if len(c.returnsStack) == 0 {
		for _, v := range r.Values {
			c.walkExpressionDiscard(v)
		}
		return
	}
	declared := c.returnsStack[len(c.returnsStack)-1]
	if declared == nil {
		for _, v := range r.Values {
			c.walkExpressionDiscard(v)
		}
		return
	}
	for i, v := range r.Values {
		got := c.typeOfExpression(v)
		if i >= len(declared) {
			c.errf(v.Line(), "extra-return",
				"function returns more values (index %d) than declared (%d)",
				i+1, len(declared))
			continue
		}
		if !assignable(got, declared[i]) {
			c.errAssign(v.Line(), got, declared[i])
		}
	}
	if len(r.Values) < len(declared) {
		for i := len(r.Values); i < len(declared); i++ {
			if !assignable(nilT, declared[i]) {
				c.errf(r.Line(), "missing-return",
					"function declared to return %d values, returning %d (slot %d expects %q)",
					len(declared), len(r.Values), i+1, declared[i].String())
			}
		}
	}
}

func (c *checker) functionShapeFromExpr(fe *ast.FunctionExpression) *Type {
	if fe == nil {
		return anyT
	}
	restore := c.pushTypeParams(fe.TypeParams)
	defer restore()

	params := make([]*Type, len(fe.Params))
	for i, p := range fe.Params {
		if p.Type != nil {
			params[i] = c.resolveAST(p.Type)
		} else {
			if c.opts.Strict {
				c.errf(fe.Line(), "implicit-any",
					"parameter %q has no type annotation (--!strict)", p.Name.Name)
			}
			params[i] = anyT
		}
		if p.Default != nil {
			got := c.typeOfExpression(p.Default)
			if !assignable(got, params[i]) {
				c.errAssign(p.Default.Line(), got, params[i])
			}
			params[i] = NewUnion(params[i], nilT)
		}
	}
	returns := make([]*Type, len(fe.ReturnTypes))
	for i, r := range fe.ReturnTypes {
		returns[i] = c.resolveAST(r)
	}
	var va *Type
	if fe.IsVararg && fe.VarargType != nil {
		va = c.resolveAST(fe.VarargType)
	}
	shape := NewFunction(params, returns, fe.IsVararg, va)
	shape.Fn.TypeParams = fe.TypeParams
	return shape
}

func (c *checker) walkFunctionBody(fe *ast.FunctionExpression, shape *FunctionShape) {
	if fe == nil || fe.Body == nil {
		return
	}
	restore := c.pushTypeParams(fe.TypeParams)
	defer restore()

	c.env.push()
	defer c.env.pop()
	for _, name := range c.env.visiblyRefinedNames() {
		if !c.assignedSomewhere[name] {
			continue
		}
		if declared, ok := c.env.lookupDeclared(name); ok {
			c.env.define(name, declared)
		}
	}
	for i, p := range fe.Params {
		bound := shape.Params[i]
		if p.Default != nil && p.Type != nil {
			bound = c.resolveAST(p.Type)
		}
		c.env.define(p.Name.Name, bound)
	}
	var declared []*Type
	if len(shape.Returns) > 0 {
		declared = shape.Returns
	}
	c.returnsStack = append(c.returnsStack, declared)
	defer func() { c.returnsStack = c.returnsStack[:len(c.returnsStack)-1] }()
	c.walkBlock(fe.Body)
}
