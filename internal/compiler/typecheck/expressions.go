package typecheck

// Expression typing.

import "github.com/hilthontt/luascript/internal/compiler/ast"

// Expression typing

// walkExpressionDiscard evaluates an expression purely for side-effect
// checking and discards the result.
func (c *checker) walkExpressionDiscard(e ast.Expression) {
	if e == nil {
		return
	}
	_ = c.typeOfExpression(e)
}

// typeOfExpression returns the inferred type of `e`. Errors are accumulated
// onto the checker; the returned Type is always non-nil so callers can
// safely use it without nil-checking. Unrecognised constructs return `any`.
func (c *checker) typeOfExpression(e ast.Expression) *Type {
	if e == nil {
		return nilT
	}
	switch n := e.(type) {
	case *ast.NilLiteral:
		return nilT
	case *ast.BooleanLiteral:
		// Literal expressions carry singleton types so they can satisfy a
		// singleton annotation (`local m: Mode = "read"`) and discriminate a
		// union. Every *inference* site widens them back to the base
		// primitive — see widen() — so untyped code is unaffected.
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
		// `expr :: T` — the assertion is the asserted type. We do walk
		// the inner expression for side-effect checking but accept any
		// runtime type (programmer-controlled cast).
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

// typeOfIfExpression types `if c then a else b`: each arm is typed with the
// preceding conditions' refinements applied (mirroring walkIfStatement), and
// the result is the union of every arm's type.
func (c *checker) typeOfIfExpression(e *ast.IfExpression) *Type {
	// Accumulator frame carries the negation of every earlier condition.
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
	// Lua tables are dynamic: callers add and remove fields at runtime
	// via `t.foo = x`. Inferring a closed shape from a literal would
	// make that idiomatic pattern fail the checker. Gradual-friendly
	// approach for v1: type-check the literal's value sub-expressions
	// for nested errors, but report the result as `any` so subsequent
	// mutations flow freely.
	//
	// Strict shape checking still applies when the surrounding slot has
	// an explicit annotation — `any` is bidirectionally compatible, so
	// `local t: {x: number} = {y = 1}` would currently NOT catch the
	// misspelling. That refinement is deferred to v2; the alternative
	// (annotated-context-aware inference) requires threading expected
	// types through the walker, which is a bigger lift than v1 deserves.
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
	// Static-key field lookup when the index is a string literal or a
	// dot-access (which the AST also models as a string key).
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
	// Dynamic key: defer to indexer if present, else any.
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
	// Any call can reach a closure that mutates a refined upvalue; the
	// narrowing must not survive it. (Args were typed above, under the
	// pre-call refinements — evaluation order matches the runtime.)
	c.invalidateCallRefinements()
	// Look through unions for a callable shape. Lua programmers routinely
	// call values of type `function | nil` (e.g. `loadfile()` results)
	// without explicit nil checks; the runtime handles the bad case.
	// V1 doesn't have refinement, so we accept any callable union member
	// and use it for arity/arg checking. If no member is callable AND the
	// callee isn't `any`, that's a real error.
	fn := callableShape(callee)
	if fn == nil {
		if callee.Kind == KindAny {
			return anyT
		}
		c.errf(call.Line(), "call-non-function",
			"cannot call a value of type %q", callee.String())
		return anyT
	}
	// Struct constructors accept a second call form: a single table literal
	// naming the fields (`Point{ x = 1, y = 2 }`). Detect it and validate
	// the table against the struct shape instead of the positional params.
	if fn.Struct != nil && c.isNamedStructCall(call) {
		c.checkNamedStructCall(call, fn.Struct)
		return fn.Returns[0]
	}
	c.checkCallArgs(call.Line(), fn, args)
	// `require("json")` with a literal name resolves to that module's type, so
	// everything reached through it is checked instead of decaying to `any`.
	if t := c.requireModuleType(call); t != nil {
		return t
	}
	if len(fn.Returns) == 0 {
		// Unannotated functions have no declared returns, but Lua callers
		// routinely use them in multi-value contexts. Returning `any`
		// (rather than `nil`) keeps gradual code unblocked.
		return anyT
	}
	// Generic call: infer the type variables from the arguments and
	// substitute them into the return type so `identity(5)` yields `number`.
	if len(fn.TypeParams) > 0 {
		return c.instantiateCall(fn, args)[0]
	}
	return fn.Returns[0]
}

// callableShape returns the FunctionShape inside `t` — directly if t is a
// function, or by scanning a union for a function member. Returns nil
// when no callable member exists. `any` returns nil too; the caller
// treats it specially.
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
	// `obj:method(args...)` — implicit self prepended. We type-check args
	// loosely (the receiver shape isn't deeply tracked in v1), but at
	// minimum we evaluate them for nested errors.
	c.walkExpressionDiscard(call.Object)
	for _, a := range call.Args {
		c.walkExpressionDiscard(a)
	}
	c.invalidateCallRefinements()
	return anyT
}

func (c *checker) checkCallArgs(line int, fn *FunctionShape, args []*Type) {
	// A declared parameter is "optional" iff nil flows to its type
	// (i.e. it's `T | nil` or `T?`). Lua callers may omit trailing
	// optional arguments — missing positions are treated as nil.
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
		// Check positional params (only those provided).
		bound := min(len(args), len(fn.Params))
		for i := 0; i < bound; i++ {
			if !assignable(args[i], fn.Params[i]) {
				c.errAssign(line, args[i], fn.Params[i])
			}
		}
		// Extras flow to vararg type.
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

// isNamedStructCall reports whether a call is the `Name{ ... }` brace form —
// a single table-literal argument. This is the syntactic signal for named
// construction; a single *variable* of table type still goes through the
// positional path (and thus an arity check), nudging users toward the
// unambiguous literal form.
func (c *checker) isNamedStructCall(call *ast.CallExpression) bool {
	if len(call.Args) != 1 {
		return false
	}
	_, ok := call.Args[0].(*ast.TableConstructor)
	return ok
}

// checkNamedStructCall validates a `Name{ field = v, ... }` construction
// against the struct shape: every field key must be declared, each value
// must be assignable to its field type, and every non-optional field must
// be present.
func (c *checker) checkNamedStructCall(call *ast.CallExpression, sc *StructCtor) {
	lit := call.Args[0].(*ast.TableConstructor)

	declared := make(map[string]*Type, len(sc.Shape.Fields))
	for _, f := range sc.Shape.Fields {
		declared[f.Key] = f.Type
	}

	provided := map[string]bool{}
	for _, f := range lit.Fields {
		// Only record-form entries (`name = v`) name a field.
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

	// and/or short-circuit, so their RHS only evaluates under the LHS's
	// truthy (and) / falsy (or) outcome — type it in a frame carrying that
	// narrowing: `s ~= nil and #s`, `x or default`.
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
		// If either operand is `any` we can't know the result statically
		// — Lua's __add / __mul / etc. metamethods can return anything,
		// so a `Vec + Vec` (both modeled as `any`) yields `any`, not a
		// number. Concrete-numeric operands give a concrete number.
		if left.Kind == KindAny || right.Kind == KindAny {
			return anyT
		}
		return numberT
	case "..":
		// Lua's concat accepts numbers (auto-stringified) and strings.
		c.requireStringLike(e.Line(), left)
		c.requireStringLike(e.Line(), right)
		if left.Kind == KindAny || right.Kind == KindAny {
			return anyT
		}
		return stringT
	case "==", "~=":
		// Equality accepts any pair.
		return booleanT
	case "<", "<=", ">", ">=":
		// Lua orders numbers and strings (within the same kind) only.
		if !sameOrderable(left, right) {
			c.errf(e.Line(), "compare-mismatch",
				"cannot compare %q with %q", left.String(), right.String())
		}
		return booleanT
	case "and":
		// `a and b` yields a only when a is falsy — the truthy members of
		// a's type can't reach the result. (false can't be split off from
		// boolean, so a boolean member survives whole.)
		falsy := keepKinds(left, KindNil, KindBoolean)
		if falsy.Kind == KindNever {
			return right
		}
		return NewUnion(falsy, right)
	case "or":
		// `a or b` yields a only when a is truthy — nil can't survive,
		// which is what makes `x or default` a non-optional.
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
		// Negation of a singleton is a singleton, so `-1` has the type `-1`
		// and can satisfy a `local n: -1` slot. (`~` is a bitwise complement
		// whose value depends on integer representation — not folded.)
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

// sameOrderable reports whether the two types are both numbers or both
// strings — the only orderings Lua's <, <=, >, >= permit.
func sameOrderable(a, b *Type) bool {
	if a.Kind == KindAny || b.Kind == KindAny {
		return true
	}
	return (assignable(a, numberT) && assignable(b, numberT)) ||
		(assignable(a, stringT) && assignable(b, stringT))
}
