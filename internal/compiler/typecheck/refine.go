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
//   - literal guards: x == "read"       /   x ~= 3   (narrows a singleton
//                    union such as `type Mode = "read" | "write"`, and the
//                    members of a classic enum)
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
	// x == <literal> — the discriminator for a singleton union. `Color.RED`
	// counts as a literal too, since a classic enum's members are typed as
	// the singleton numbers they are.
	if name, ok := identName(a); ok {
		if lit := c.staticLiteralType(b); lit != nil {
			return c.refineLiteralGuard(name, lit, eq)
		}
	}
	if name, ok := identName(b); ok {
		if lit := c.staticLiteralType(a); lit != nil {
			return c.refineLiteralGuard(name, lit, eq)
		}
	}
	return nil
}

// staticLiteralType returns the singleton type of an expression whose value
// is knowable without evaluating anything — a literal, or a dotted read of a
// field that is itself typed as a singleton (`Color.RED`, `Mode.Read`).
// Returns nil for everything else.
//
// Deliberately does NOT go through typeOfExpression: refine() must stay
// side-effect free, and typeOfExpression records errors.
func (c *checker) staticLiteralType(e ast.Expression) *Type {
	switch n := e.(type) {
	case *ast.ParenExpression:
		return c.staticLiteralType(n.Inner)
	case *ast.StringLiteral:
		return NewStringLiteral(n.Value, "")
	case *ast.IntegerLiteral:
		return NewNumberLiteral(float64(n.Value), "")
	case *ast.FloatLiteral:
		return NewNumberLiteral(n.Value, "")
	case *ast.BooleanLiteral:
		return NewBooleanLiteral(n.Value, "")
	case *ast.IndexExpression:
		base, ok := n.Object.(*ast.Identifier)
		if !ok {
			return nil
		}
		name, ok := staticIndexName(n)
		if !ok {
			return nil
		}
		bt, ok := c.env.lookup(base.Name)
		if !ok || bt == nil || bt.Kind != KindTable || bt.Table == nil {
			return nil
		}
		for _, f := range bt.Table.Fields {
			if f.Key == name && f.Type != nil && f.Type.Kind == KindLiteral {
				return f.Type
			}
		}
	}
	return nil
}

// refineLiteralGuard narrows `name` on an `x == <literal>` test. `eq` true →
// the equality holds, so x is exactly that singleton; false → x is everything
// in its type *except* that singleton, which is only representable when the
// type is a union of literals.
func (c *checker) refineLiteralGuard(name string, lit *Type, eq bool) refinement {
	t, ok := c.env.lookup(name)
	if !ok || t == nil {
		return nil
	}
	var nt *Type
	if eq {
		nt = narrowToLiteral(t, lit)
	} else {
		nt = removeLiteral(t, lit)
	}
	if nt == nil || Same(nt, t) {
		return nil
	}
	return refinement{name: nt}
}

// narrowToLiteral keeps only the part of `t` that can equal `lit`.
//
//   - `any`/`unknown` refine to the singleton, matching how a type guard
//     refines the gradual top.
//   - a union keeps the member equal to `lit`.
//   - a base primitive of the right kind refines to the singleton
//     (`string` under `s == "read"` becomes `"read"`).
//   - anything else collapses to `never` — an impossible branch.
func narrowToLiteral(t, lit *Type) *Type {
	if t == nil || lit == nil {
		return nil
	}
	switch t.Kind {
	case KindAny, KindUnknown:
		return lit
	case KindUnion:
		kept := make([]*Type, 0, 1)
		for _, m := range t.Union {
			if Same(m, lit) || (m.Kind != KindLiteral && m.Kind == baseKind(lit)) {
				kept = append(kept, lit)
				break
			}
		}
		return NewUnion(kept...)
	}
	if Same(t, lit) {
		return t
	}
	if t.Kind == baseKind(lit) {
		return lit
	}
	return neverT
}

// removeLiteral drops the member of `t` equal to `lit`. Only a union of
// literals can express the result — "every string except \"read\"" is not a
// type we can represent, so a base primitive is returned unchanged.
func removeLiteral(t, lit *Type) *Type {
	if t == nil || lit == nil {
		return nil
	}
	if t.Kind == KindUnion {
		kept := make([]*Type, 0, len(t.Union))
		for _, m := range t.Union {
			if !Same(m, lit) {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if Same(t, lit) {
		return neverT
	}
	return t
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
			if baseKind(m) == k {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if baseKind(t) == k {
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
			if baseKind(m) != k {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if baseKind(t) == k {
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
			if want(baseKind(m)) {
				kept = append(kept, m)
			}
		}
		return NewUnion(kept...)
	}
	if want(baseKind(t)) {
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
//     assigns the refined variable as an upvalue (the call may reach it).

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
