package typecheck

// Type refinement (a.k.a. occurrence typing / narrowing).
//
// When the checker enters a conditional branch it knows more about the
// program than the surrounding scope does: inside `if type(x) == "string"
// then ... end`, the variable `x` is a `string` regardless of its wider
// declared type. This file computes those narrowings and the if/while
// walkers install them into a pushed env frame for the duration of the
// branch.
//
// Scope of v1 (the discriminators that cover the overwhelming majority of
// real Lua):
//
//   - type guards:   type(x) == "T"   /   typeof(x) == "T"   (and ~=)
//   - nil guards:    x == nil          /   x ~= nil
//   - truthiness:    if x then ...     (then-branch: x is non-nil)
//   - negation:      not <cond>        (swaps the then/else narrowing)
//   - and / or:      conjunction propagates to the then-branch; disjunction
//                    propagates to the else-branch (the soundly-decidable
//                    halves of De Morgan)
//   - early exit:    `if s == nil then return end` — when a leading prefix
//                    of if-clauses always terminates (return/break/continue/
//                    throw/error()), their negations persist after the `end`
//   - assert:        `assert(cond)` applies cond's positive narrowing to the
//                    rest of the block
//   - expressions:   the RHS of `a and b` is typed under a's positive
//                    narrowing; the RHS of `a or b` under its negative
//
// Assignment interplay: narrowing shadows are marked in the env (see
// defineRefined) so an assignment is checked against the *declared* type and
// then widens every shadow with the assigned type — a refinement never
// outlives the value it described.
//
// Only simple identifiers are refinable — field paths like `x.y` are not
// tracked (they can be invalidated by aliasing, and Luau itself only
// refines them under tighter conditions). Everything narrows by *shadowing*
// in a child env frame, so a refinement never escapes its branch and the
// env model needs no changes.

import (
	"maps"
	"slices"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// refinement maps a local name to the type it should hold inside a branch.
// An empty/nil refinement means "nothing learned" — the branch sees the
// outer types unchanged.
type refinement map[string]*Type

// refine computes the narrowing implied by `cond` evaluating truthy
// (positive == true) or falsy (positive == false). It is side-effect free:
// it reads current types via env.lookup and never records errors, so the
// caller is responsible for walking the condition once for error checking.
func (c *checker) refine(cond ast.Expression, positive bool) refinement {
	switch n := cond.(type) {
	case *ast.ParenExpression:
		return c.refine(n.Inner, positive)

	case *ast.UnaryExpression:
		if n.Op == "not" {
			// `not e` is truthy exactly when `e` is falsy: flip polarity.
			return c.refine(n.Operand, !positive)
		}

	case *ast.Identifier:
		return c.refineTruthiness(n.Name, positive)

	case *ast.BinaryExpression:
		return c.refineBinary(n, positive)
	}
	return nil
}

// refineTruthiness handles a bare `if x then` / `else`.
//
//   - then-branch (positive): x is truthy, so it cannot be nil. We drop the
//     nil member. (We can't drop `false`, since there is no false-singleton
//     type, and `true` keeps boolean live — so a boolean member stays.)
//   - else-branch (negative): x is falsy. The only falsy values we can
//     represent are nil and boolean (the `false` half). We keep just those
//     members; if that leaves nothing representable we learn nothing.
func (c *checker) refineTruthiness(name string, positive bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	if positive {
		nt := removeKind(t, KindNil)
		if Same(nt, t) {
			return nil
		}
		return refinement{name: nt}
	}
	falsy := keepKinds(t, KindNil, KindBoolean)
	if falsy.Kind == KindNever || Same(falsy, t) {
		// Either an impossible else (e.g. a plain `string` that can never be
		// falsy) or no information gained — leave the type alone rather than
		// poison the branch with `never`.
		return nil
	}
	return refinement{name: falsy}
}

// refineBinary dispatches on the operator of a boolean-valued binary.
func (c *checker) refineBinary(n *ast.BinaryExpression, positive bool) refinement {
	switch n.Op {
	case "and":
		// (a and b) is truthy iff both are truthy → propagate both then-maps.
		// Its falsy case (a falsy OR b falsy) can't be pinned to one side.
		if positive {
			return mergeRefine(c.refine(n.Left, true), c.refine(n.Right, true))
		}
		return nil

	case "or":
		// (a or b) is falsy iff both are falsy → propagate both else-maps.
		// Its truthy case can't be pinned to one side.
		if !positive {
			return mergeRefine(c.refine(n.Left, false), c.refine(n.Right, false))
		}
		return nil

	case "==", "~=":
		// Effective equality after folding in branch polarity:
		//   ==  in then-branch  → testing equality holds
		//   ~=  in then-branch  → testing equality fails
		//   either, negated, flips again.
		eq := (n.Op == "==") == positive
		return c.refineEquality(n.Left, n.Right, eq)
	}
	return nil
}

// refineEquality recognises the two equality shapes that carry type
// information, in either operand order:
//
//	type(x) == "T"   /   typeof(x) == "T"
//	x == nil
func (c *checker) refineEquality(a, b ast.Expression, eq bool) refinement {
	// type(x) == "T" — trusted only while `type`/`typeof` still denotes the
	// builtin; a shadowing local of the same name proves nothing.
	if name, guard, ok := typeGuardTarget(a); ok && c.builtinInScope(guard) {
		if lit, ok := stringLitValue(b); ok {
			return c.refineTypeGuard(name, lit, eq)
		}
	}
	if name, guard, ok := typeGuardTarget(b); ok && c.builtinInScope(guard) {
		if lit, ok := stringLitValue(a); ok {
			return c.refineTypeGuard(name, lit, eq)
		}
	}
	// x == nil
	if name, ok := identName(a); ok && isNilLiteral(b) {
		return c.refineNilGuard(name, eq)
	}
	if name, ok := identName(b); ok && isNilLiteral(a) {
		return c.refineNilGuard(name, eq)
	}
	return nil
}

// refineTypeGuard narrows `name` based on a `type(name) == "kind"` test.
// `eq` is true when the equality holds in this branch (narrow *to* the
// kind), false when it fails (narrow *away from* the kind).
func (c *checker) refineTypeGuard(name, kindStr string, eq bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	k, ok := kindForTypeString(kindStr)
	if !ok {
		// "thread"/"userdata"/unknown — not modeled by the type system, so
		// we can't represent the narrowing. Learn nothing.
		return nil
	}
	var nt *Type
	if eq {
		nt = narrowToKind(t, k)
	} else {
		nt = removeKind(t, k)
	}
	if Same(nt, t) {
		return nil
	}
	return refinement{name: nt}
}

// refineNilGuard narrows `name` based on an `x == nil` test. `eq` true →
// the equality holds (x is nil); false → it fails (x is non-nil).
func (c *checker) refineNilGuard(name string, eq bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	var nt *Type
	if eq {
		nt = narrowToKind(t, KindNil)
	} else {
		nt = removeKind(t, KindNil)
	}
	if Same(nt, t) {
		return nil
	}
	return refinement{name: nt}
}

// applyRefinement installs a refinement into the current (innermost) env
// frame, shadowing outer bindings for the lifetime of that frame. The
// bindings are marked as refinement shadows so assignment checking can see
// through them to the declared type.
func (c *checker) applyRefinement(r refinement) {
	for name, t := range r {
		c.env.defineRefined(name, t)
	}
}

// mergeRefine combines two refinements. When both touch the same name the
// second wins; in practice the two sides of an `and`/`or` almost always
// refer to different variables, so this is rarely exercised.
func mergeRefine(a, b refinement) refinement {
	switch {
	case len(a) == 0:
		return b
	case len(b) == 0:
		return a
	}
	out := make(refinement, len(a)+len(b))
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

// Narrowing operations on types

// narrowToKind keeps only the part of `t` whose Kind is `k`.
//
//   - `any`/`unknown` refine to the guarded primitive (Luau treats a type
//     guard as evidence strong enough to refine the gradual top). When the
//     guarded kind has no singleton (table/function) the value is left as-is
//     because there's no shape to substitute.
//   - a union keeps its matching members.
//   - a non-matching concrete type collapses to `never` (an impossible
//     branch); `never` is permissive downstream, so this won't manufacture
//     spurious errors.
func narrowToKind(t *Type, k Kind) *Type {
	if t == nil {
		return t
	}
	switch t.Kind {
	case KindAny, KindUnknown:
		if p := primitiveForKind(k); p != nil {
			return p
		}
		return t
	case KindUnion:
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if m.Kind == k {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if t.Kind == k {
		return t
	}
	return neverT
}

// removeKind drops every member of `t` whose Kind is `k`.
//
//   - `any`/`unknown` are left untouched: they can hold a value of any kind,
//     so "this isn't a string" tells us nothing representable.
//   - a union drops its matching members.
//   - a concrete type equal to `k` collapses to `never`.
func removeKind(t *Type, k Kind) *Type {
	if t == nil || t.Kind == KindAny || t.Kind == KindUnknown {
		return t
	}
	if t.Kind == KindUnion {
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if m.Kind != k {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if t.Kind == k {
		return neverT
	}
	return t
}

// keepKinds keeps only the members of `t` whose Kind is in `kinds`. Used by
// the falsy-branch of a truthiness test (keep nil + boolean). On `any`/
// `unknown` it learns nothing and returns the input unchanged.
func keepKinds(t *Type, kinds ...Kind) *Type {
	if t == nil || t.Kind == KindAny || t.Kind == KindUnknown {
		return t
	}
	want := func(k Kind) bool {
		return slices.Contains(kinds, k)
	}
	if t.Kind == KindUnion {
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if want(m.Kind) {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if want(t.Kind) {
		return t
	}
	return neverT
}

// primitiveForKind returns the singleton primitive Type for a Kind, or nil
// for kinds that have no canonical singleton (function/table/union/etc.).
func primitiveForKind(k Kind) *Type {
	switch k {
	case KindNumber:
		return numberT
	case KindString:
		return stringT
	case KindBoolean:
		return booleanT
	case KindNil:
		return nilT
	}
	return nil
}

// kindForTypeString maps a Lua `type()` result string to a checker Kind.
// The unmodeled runtime kinds ("thread", "userdata") return ok == false.
func kindForTypeString(s string) (Kind, bool) {
	switch s {
	case "number":
		return KindNumber, true
	case "string":
		return KindString, true
	case "boolean":
		return KindBoolean, true
	case "nil":
		return KindNil, true
	case "table":
		return KindTable, true
	case "function":
		return KindFunction, true
	}
	return 0, false
}

// Terminator analysis (early-exit narrowing)

// blockTerminates reports whether a block never falls through: every path
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
		switch n := s.(type) {
		case *ast.ContinueStatement:
			return true
		case *ast.DoStatement:
			if blockHasDirectContinue(n.Body) {
				return true
			}
		case *ast.TryCatchStatement:
			if blockHasDirectContinue(n.Try) || blockHasDirectContinue(n.Catch) {
				return true
			}
		case *ast.IfStatement:
			for _, cl := range n.Clauses {
				if blockHasDirectContinue(cl.Body) {
					return true
				}
			}
			if blockHasDirectContinue(n.Else) {
				return true
			}
		}
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
//     assigns the refined variable as an upvalue (the call may reach it).
//
// scanMutations walks the program once and fills the checker's two
// name sets. Matching is by name (not by binding identity), which
// over-approximates in the presence of same-named distinct variables — the
// conservative direction: at worst a valid narrowing is dropped.

// scanMutations populates c.assignedSomewhere and c.upvalMutated from the
// program body.
func (c *checker) scanMutations(b *ast.Block) {
	c.assignedSomewhere = map[string]bool{}
	c.upvalMutated = map[string]bool{}
	scanBlockMutations(b, c.assignedSomewhere, c.upvalMutated, false, nil)
}

// scanBlockMutations records every identifier assigned in the subtree into
// `assigned`. When `insideFn` is true, an assigned name that is not among
// the enclosing function literal's own declarations (`fnLocals`) is also an
// upvalue mutation. fnLocals is a name set accumulated per function literal;
// block scoping inside one function is deliberately ignored (name-based
// approximation).
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
	case *ast.TryCatchStatement:
		scanBlockMutations(n.Try, assigned, upval, insideFn, fnLocals)
		scanBlockMutations(n.Catch, assigned, upval, insideFn, fnLocals)
	case *ast.ThrowStatement:
		recurseExpr(n.Value)
	case *ast.DeferStatement:
		recurseExpr(n.Call)
	}
}

// scanFnMutations enters a function literal: its body scans with a fresh
// local-name set seeded with the parameters, and everything assigned there
// that isn't function-local counts as an upvalue mutation.
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

// scanExprMutations descends into expressions only to find nested function
// literals (the one expression form that contains statements).
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

// AST shape helpers

// typeGuardTarget returns the identifier name `x` when `e` is a call of the
// form `type(x)` or `typeof(x)` with a single identifier argument, plus the
// guard function's own name so the caller can verify it still denotes the
// builtin.
func typeGuardTarget(e ast.Expression) (target, guardFn string, ok bool) {
	call, isCall := e.(*ast.CallExpression)
	if !isCall || len(call.Args) != 1 {
		return "", "", false
	}
	fn, isIdent := call.Func.(*ast.Identifier)
	if !isIdent || (fn.Name != "type" && fn.Name != "typeof") {
		return "", "", false
	}
	target, ok = identName(call.Args[0])
	return target, fn.Name, ok
}

// identName returns the name of an identifier expression (peeling a
// redundant paren), or ok == false for anything else.
func identName(e ast.Expression) (string, bool) {
	if p, ok := e.(*ast.ParenExpression); ok {
		return identName(p.Inner)
	}
	if id, ok := e.(*ast.Identifier); ok {
		return id.Name, true
	}
	return "", false
}

// stringLitValue returns the value of a string-literal expression.
func stringLitValue(e ast.Expression) (string, bool) {
	if p, ok := e.(*ast.ParenExpression); ok {
		return stringLitValue(p.Inner)
	}
	if s, ok := e.(*ast.StringLiteral); ok {
		return s.Value, true
	}
	return "", false
}

// isNilLiteral reports whether `e` is the `nil` literal.
func isNilLiteral(e ast.Expression) bool {
	if p, ok := e.(*ast.ParenExpression); ok {
		return isNilLiteral(p.Inner)
	}
	_, ok := e.(*ast.NilLiteral)
	return ok
}
