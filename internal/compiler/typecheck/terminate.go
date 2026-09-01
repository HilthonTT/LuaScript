package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

func (c *checker) blockTerminates(b *ast.Block) bool {
	if b == nil {
		return false
	}
	if blockContainsGoto(b) {
		return false
	}
	return c.blockTerminatesNoGoto(b)
}

func (c *checker) blockTerminatesNoGoto(b *ast.Block) bool {
	if b == nil {
		return false
	}
	if b.Return != nil {
		return true
	}
	if len(b.Statements) == 0 {
		return false
	}
	return c.statementTerminates(b.Statements[len(b.Statements)-1])
}

func (c *checker) statementTerminates(s ast.Statement) bool {
	switch n := s.(type) {
	case *ast.ReturnStatement, *ast.BreakStatement, *ast.ContinueStatement,
		*ast.ThrowStatement:
		return true
	case *ast.ExpressionStatement:
		return c.isErrorCall(n.Expression)
	case *ast.DoStatement:
		return c.blockTerminatesNoGoto(n.Body)
	case *ast.IfStatement:
		if n.Else == nil || !c.blockTerminatesNoGoto(n.Else) {
			return false
		}
		for _, cl := range n.Clauses {
			if !c.blockTerminatesNoGoto(cl.Body) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *checker) isErrorCall(e ast.Expression) bool {
	call, ok := e.(*ast.CallExpression)
	if !ok {
		return false
	}
	fn, ok := call.Func.(*ast.Identifier)
	return ok && fn.Name == "error" && c.builtinInScope("error")
}

func blockContainsGoto(b *ast.Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Statements {
		if statementContainsGoto(s) {
			return true
		}
	}
	return false
}

func statementContainsGoto(s ast.Statement) bool {
	switch n := s.(type) {
	case *ast.GotoStatement:
		return true
	case *ast.DoStatement:
		return blockContainsGoto(n.Body)
	case *ast.WhileStatement:
		return blockContainsGoto(n.Body)
	case *ast.RepeatStatement:
		return blockContainsGoto(n.Body)
	case *ast.NumericForStatement:
		return blockContainsGoto(n.Body)
	case *ast.GenericForStatement:
		return blockContainsGoto(n.Body)
	case *ast.MatchStatement:
		for i := range n.Arms {
			if statementContainsGoto(n.Arms[i].Body) {
				return true
			}
		}
		return false
	case *ast.TryCatchStatement:
		return blockContainsGoto(n.Try) || blockContainsGoto(n.Catch)
	case *ast.IfStatement:
		for _, cl := range n.Clauses {
			if blockContainsGoto(cl.Body) {
				return true
			}
		}
		return blockContainsGoto(n.Else)
	}
	return false
}

func blockHasDirectContinue(b *ast.Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Statements {
		if stmtHasDirectContinue(s) {
			return true
		}
	}
	return false
}

func stmtHasDirectContinue(s ast.Statement) bool {
	switch n := s.(type) {
	case *ast.ContinueStatement:
		return true
	case *ast.DoStatement:
		return blockHasDirectContinue(n.Body)
	case *ast.TryCatchStatement:
		return blockHasDirectContinue(n.Try) || blockHasDirectContinue(n.Catch)
	case *ast.MatchStatement:
		for i := range n.Arms {
			if stmtHasDirectContinue(n.Arms[i].Body) {
				return true
			}
		}
		return false
	case *ast.IfStatement:
		for _, cl := range n.Clauses {
			if blockHasDirectContinue(cl.Body) {
				return true
			}
		}
		return blockHasDirectContinue(n.Else)
	}
	return false
}

func assertCondition(e ast.Expression) (ast.Expression, bool) {
	call, ok := e.(*ast.CallExpression)
	if !ok || len(call.Args) < 1 {
		return nil, false
	}
	fn, ok := call.Func.(*ast.Identifier)
	if !ok || fn.Name != "assert" {
		return nil, false
	}
	return call.Args[0], true
}
