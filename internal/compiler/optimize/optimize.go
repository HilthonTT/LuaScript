package optimize

import "github.com/hilthontt/luascript/internal/compiler/ast"

// Fold rewrites prog in place, replacing constant expressions with literals.
// It is safe to call unconditionally; on a program with no foldable
// expressions it leaves the tree unchanged.
func Fold(prog *ast.Program) {
	if prog == nil {
		return
	}
	foldBlock(prog.Block)
}

func foldBlock(b *ast.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Statements {
		foldStmt(s)
	}
	if b.Return != nil {
		foldStmt(b.Return)
	}
}

func foldStmt(s ast.Statement) {
	switch n := s.(type) {
	case *ast.AssignStatement:
		// Targets are Identifiers / IndexExpressions; folding only rewrites
		// their sub-expressions (e.g. the index of t[1+1]), never the target.
		foldExprSlice(n.Targets)
		foldExprSlice(n.Values)
	case *ast.LocalStatement:
		foldExprSlice(n.Values)
	case *ast.LocalFunctionStatement:
		foldFunc(n.Func)
	case *ast.FunctionDeclaration:
		foldFunc(n.Func)
	case *ast.IfStatement:
		for i := range n.Clauses {
			n.Clauses[i].Condition = foldExpr(n.Clauses[i].Condition)
			foldBlock(n.Clauses[i].Body)
		}
		foldBlock(n.Else)
	case *ast.WhileStatement:
		n.Condition = foldExpr(n.Condition)
		foldBlock(n.Body)
	case *ast.RepeatStatement:
		foldBlock(n.Body)
		n.Condition = foldExpr(n.Condition)
	case *ast.NumericForStatement:
		n.Start = foldExpr(n.Start)
		n.Limit = foldExpr(n.Limit)
		if n.Step != nil {
			n.Step = foldExpr(n.Step)
		}
		foldBlock(n.Body)
	case *ast.GenericForStatement:
		foldExprSlice(n.Exprs)
		foldBlock(n.Body)
	case *ast.DoStatement:
		foldBlock(n.Body)
	case *ast.ReturnStatement:
		foldExprSlice(n.Values)
	case *ast.ExpressionStatement:
		if n.Expression != nil {
			n.Expression = foldExpr(n.Expression)
		}
	case *ast.DeferStatement:
		if n.Call != nil {
			n.Call = foldExpr(n.Call)
		}
	case *ast.MatchStatement:
		n.Subject = foldExpr(n.Subject)
		for i := range n.Arms {
			arm := &n.Arms[i]
			// Only value patterns hold expressions; the other kinds carry
			// names and type nodes, which fold to themselves.
			foldExprSlice(arm.Pattern.Values)
			if arm.Guard != nil {
				arm.Guard = foldExpr(arm.Guard)
			}
			foldStmt(arm.Body)
		}
	case *ast.TryCatchStatement:
		foldBlock(n.Try)
		foldBlock(n.Catch)
	case *ast.ThrowStatement:
		if n.Value != nil {
			n.Value = foldExpr(n.Value)
		}
	case *ast.Block:
		foldBlock(n)
	}
	// BreakStatement, ContinueStatement, GotoStatement, LabelStatement,
	// TypeAliasStatement carry no foldable expressions.
}

// foldFunc folds a function literal's parameter defaults and body.
func foldFunc(fe *ast.FunctionExpression) {
	if fe == nil {
		return
	}
	for i := range fe.Params {
		if fe.Params[i].Default != nil {
			fe.Params[i].Default = foldExpr(fe.Params[i].Default)
		}
	}
	foldBlock(fe.Body)
}

func foldExprSlice(es []ast.Expression) {
	for i := range es {
		es[i] = foldExpr(es[i])
	}
}

// foldExpr folds e bottom-up and returns the (possibly new) node. Callers
// MUST reassign the slot they passed in.
func foldExpr(e ast.Expression) ast.Expression {
	switch n := e.(type) {
	case *ast.BinaryExpression:
		n.Left = foldExpr(n.Left)
		n.Right = foldExpr(n.Right)
		if folded := tryFoldBinary(n); folded != nil {
			return folded
		}
		return n
	case *ast.UnaryExpression:
		n.Operand = foldExpr(n.Operand)
		if folded := tryFoldUnary(n); folded != nil {
			return folded
		}
		return n
	case *ast.ParenExpression:
		n.Inner = foldExpr(n.Inner)
		// A literal is already single-valued, so the parens' multi-value
		// truncation is a no-op and can be dropped.
		if isLiteral(n.Inner) {
			return n.Inner
		}
		return n
	case *ast.CallExpression:
		n.Func = foldExpr(n.Func)
		foldExprSlice(n.Args)
		return n
	case *ast.MethodCallExpression:
		n.Object = foldExpr(n.Object)
		foldExprSlice(n.Args)
		return n
	case *ast.IndexExpression:
		n.Object = foldExpr(n.Object)
		n.Index = foldExpr(n.Index)
		return n
	case *ast.TableConstructor:
		for i := range n.Fields {
			if n.Fields[i].Key != nil {
				n.Fields[i].Key = foldExpr(n.Fields[i].Key)
			}
			n.Fields[i].Value = foldExpr(n.Fields[i].Value)
		}
		return n
	case *ast.FunctionExpression:
		foldFunc(n)
		return n
	case *ast.IfExpression:
		return foldIfExpr(n)
	case *ast.MatchExpression:
		n.Subject = foldExpr(n.Subject)
		for i := range n.Arms {
			if n.Arms[i].Guard != nil {
				n.Arms[i].Guard = foldExpr(n.Arms[i].Guard)
			}
			n.Arms[i].Body = foldExpr(n.Arms[i].Body)
		}
		return n
	case *ast.TypeAssertionExpression:
		n.Expr = foldExpr(n.Expr)
		return n
	default:
		// Literals, Identifier, VarargExpression: nothing to fold.
		return e
	}
}

func isLiteral(e ast.Expression) bool {
	switch e.(type) {
	case *ast.NilLiteral, *ast.BooleanLiteral, *ast.IntegerLiteral,
		*ast.FloatLiteral, *ast.StringLiteral:
		return true
	}
	return false
}

func isTruthy(e ast.Expression) bool {
	switch n := e.(type) {
	case *ast.NilLiteral:
		return false
	case *ast.BooleanLiteral:
		return n.Value
	}
	// Every other literal (numbers, strings) is truthy in Lua.
	return true
}
