package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) scanMutations(b *ast.Block) {
	c.assignedSomewhere = map[string]bool{}
	c.upvalMutated = map[string]bool{}
	scanBlockMutations(b, c.assignedSomewhere, c.upvalMutated, false, nil)
}

func scanBlockMutations(b *ast.Block, assigned, upval map[string]bool, insideFn bool, fnLocals map[string]bool) {
	if b == nil {
		return
	}
	for _, s := range b.Statements {
		scanStatementMutations(s, assigned, upval, insideFn, fnLocals)
	}
}

func scanStatementMutations(s ast.Statement, assigned, upval map[string]bool, insideFn bool, fnLocals map[string]bool) {
	recurseExpr := func(e ast.Expression) {
		scanExprMutations(e, assigned, upval, insideFn, fnLocals)
	}
	switch n := s.(type) {
	case *ast.AssignStatement:
		for _, t := range n.Targets {
			if name, ok := identName(t); ok {
				assigned[name] = true
				if insideFn && !fnLocals[name] {
					upval[name] = true
				}
			}
		}
		for _, v := range n.Values {
			recurseExpr(v)
		}
	case *ast.LocalStatement:
		for _, name := range n.Names {
			if insideFn {
				fnLocals[name.Name] = true
			}
		}
		for _, v := range n.Values {
			recurseExpr(v)
		}
	case *ast.LocalFunctionStatement:
		if insideFn {
			fnLocals[n.Name] = true
		}
		scanFnMutations(n.Func, assigned, upval)
	case *ast.FunctionDeclaration:
		scanFnMutations(n.Func, assigned, upval)
	case *ast.ExpressionStatement:
		recurseExpr(n.Expression)
	case *ast.ReturnStatement:
		for _, v := range n.Values {
			recurseExpr(v)
		}
	case *ast.DoStatement:
		scanBlockMutations(n.Body, assigned, upval, insideFn, fnLocals)
	case *ast.WhileStatement:
		recurseExpr(n.Condition)
		scanBlockMutations(n.Body, assigned, upval, insideFn, fnLocals)
	case *ast.RepeatStatement:
		scanBlockMutations(n.Body, assigned, upval, insideFn, fnLocals)
		recurseExpr(n.Condition)
	case *ast.NumericForStatement:
		recurseExpr(n.Start)
		recurseExpr(n.Limit)
		recurseExpr(n.Step)
		scanBlockMutations(n.Body, assigned, upval, insideFn, fnLocals)
	case *ast.GenericForStatement:
		for _, e := range n.Exprs {
			recurseExpr(e)
		}
		scanBlockMutations(n.Body, assigned, upval, insideFn, fnLocals)
	case *ast.IfStatement:
		for _, cl := range n.Clauses {
			recurseExpr(cl.Condition)
			scanBlockMutations(cl.Body, assigned, upval, insideFn, fnLocals)
		}
		scanBlockMutations(n.Else, assigned, upval, insideFn, fnLocals)
	case *ast.MatchStatement:
		recurseExpr(n.Subject)
		for i := range n.Arms {
			arm := &n.Arms[i]
			for _, v := range arm.Pattern.Values {
				recurseExpr(v)
			}
			if arm.Guard != nil {
				recurseExpr(arm.Guard)
			}
			scanStatementMutations(arm.Body, assigned, upval, insideFn, fnLocals)
		}
	case *ast.TryCatchStatement:
		scanBlockMutations(n.Try, assigned, upval, insideFn, fnLocals)
		scanBlockMutations(n.Catch, assigned, upval, insideFn, fnLocals)
	case *ast.ThrowStatement:
		recurseExpr(n.Value)
	case *ast.DeferStatement:
		recurseExpr(n.Call)
	}
}

func scanFnMutations(fe *ast.FunctionExpression, assigned, upval map[string]bool) {
	if fe == nil {
		return
	}
	locals := map[string]bool{}
	for _, p := range fe.Params {
		locals[p.Name.Name] = true
	}
	scanBlockMutations(fe.Body, assigned, upval, true, locals)
}

func scanExprMutations(e ast.Expression, assigned, upval map[string]bool, insideFn bool, fnLocals map[string]bool) {
	switch n := e.(type) {
	case *ast.FunctionExpression:
		scanFnMutations(n, assigned, upval)
	case *ast.ParenExpression:
		scanExprMutations(n.Inner, assigned, upval, insideFn, fnLocals)
	case *ast.UnaryExpression:
		scanExprMutations(n.Operand, assigned, upval, insideFn, fnLocals)
	case *ast.BinaryExpression:
		scanExprMutations(n.Left, assigned, upval, insideFn, fnLocals)
		scanExprMutations(n.Right, assigned, upval, insideFn, fnLocals)
	case *ast.CallExpression:
		scanExprMutations(n.Func, assigned, upval, insideFn, fnLocals)
		for _, a := range n.Args {
			scanExprMutations(a, assigned, upval, insideFn, fnLocals)
		}
	case *ast.MethodCallExpression:
		scanExprMutations(n.Object, assigned, upval, insideFn, fnLocals)
		for _, a := range n.Args {
			scanExprMutations(a, assigned, upval, insideFn, fnLocals)
		}
	case *ast.IndexExpression:
		scanExprMutations(n.Object, assigned, upval, insideFn, fnLocals)
		scanExprMutations(n.Index, assigned, upval, insideFn, fnLocals)
	case *ast.TableConstructor:
		for _, f := range n.Fields {
			scanExprMutations(f.Key, assigned, upval, insideFn, fnLocals)
			scanExprMutations(f.Value, assigned, upval, insideFn, fnLocals)
		}
	case *ast.IfExpression:
		for _, cl := range n.Clauses {
			scanExprMutations(cl.Condition, assigned, upval, insideFn, fnLocals)
			scanExprMutations(cl.Value, assigned, upval, insideFn, fnLocals)
		}
		scanExprMutations(n.Else, assigned, upval, insideFn, fnLocals)
	case *ast.TypeAssertionExpression:
		scanExprMutations(n.Expr, assigned, upval, insideFn, fnLocals)
	}
}
