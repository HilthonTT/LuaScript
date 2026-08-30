package typecheck

// Control-flow analysis: which blocks terminate, and where goto/continue appear.

import "github.com/hilthontt/luascript/internal/compiler/ast"

// through it ends in return/break/continue/throw or a call to error().
// Used by walkIfStatement to persist a clause's negation past the `end` —
// `if s == nil then return end` leaves s non-nil for the rest of the block.
//
// goto is not a terminator — and worse, a goto *anywhere* in the block can
// escape to a label after the `end` from a path that never reaches the
// block's terminating last statement, so any goto disqualifies the whole
// block rather than just its final position.
func (c *checker) blockTerminates(b *ast.Block) bool {
	if b == nil {
		return false
	}
	if blockContainsGoto(b) {
		return false
	}
	return c.blockTerminatesNoGoto(b)
}

// blockTerminatesNoGoto is blockTerminates without the (recursive) goto
// scan, which the entry point has already performed for the whole subtree.
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
		// An if terminates only when every arm does — which requires an
		// else, or the fall-through path escapes.
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

// isErrorCall matches a bare `error(...)` call statement — but only while
// `error` still denotes the builtin: a shadowing local that happens to be
// named error need not terminate.
func (c *checker) isErrorCall(e ast.Expression) bool {
	call, ok := e.(*ast.CallExpression)
	if !ok {
		return false
	}
	fn, ok := call.Func.(*ast.Identifier)
	return ok && fn.Name == "error" && c.builtinInScope("error")
}

// blockContainsGoto reports whether any statement in the block subtree is a
// goto. Function-literal bodies are not entered: a goto cannot cross a
// function boundary, so their gotos are irrelevant to this block.
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

// blockHasDirectContinue reports whether the block contains a `continue`
// belonging to the enclosing loop — i.e. not inside a nested loop (whose
// continue targets that loop) or a function literal.
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

// stmtHasDirectContinue is blockHasDirectContinue's per-statement half. It
// exists as its own function because a match arm's body is a single
// statement rather than a block.
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

// assertCondition returns the condition argument when `e` is a call of the
// form `assert(cond, ...)`. The statement walker applies cond's positive
// narrowing to the rest of the block: control only continues past an assert
// that held.
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

// Mutation pre-scan
//
// Two soundness holes need whole-program knowledge of assignments:
//
//   - a closure created inside a narrowed branch must not keep the narrowed
//     type if the variable can be reassigned (the closure may run after the
//     mutation), and
//   - a refinement must not survive a function call when some closure
