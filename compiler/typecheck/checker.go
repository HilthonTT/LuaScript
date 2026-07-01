package typecheck

import (
	"github.com/hilthontt/luascript/compiler/ast"
)

// Options controls checker behavior. Default zero value = nonstrict
// (Luau-style gradual). Set Strict to enforce annotations on params.
type Options struct {
	// Strict: report unannotated function parameters and missing alias
	// resolutions as errors (instead of falling back to `any`).
	Strict bool
}

// Check runs the type checker over `prog` and returns the accumulated
// errors. The error list is sorted by source line. An empty slice means
// the program type-checks under the supplied options.
func Check(prog *ast.Program, opts Options) []TypeError {
	c := &checker{env: newEnv(), opts: opts}
	c.installGlobals()
	if prog == nil || prog.Block == nil {
		return nil
	}
	c.preResolveAliases(prog.Block.Statements)
	c.walkBlock(prog.Block)
	sortByLine(c.errors)
	return c.errors
}

// checker is the per-pass walker state. One Check call → one checker.
type checker struct {
	env  *env
	opts Options

	errors []TypeError

	// returnsStack tracks the declared return types of the function we're
	// currently walking. Pushed on function entry, popped on exit. Top of
	// the stack is consulted when checking ReturnStatement. nil means
	// "no return type declared" — return statements in such functions
	// flow no constraints.
	returnsStack [][]*Type

	// typeParamScopes is a stack of in-scope generic parameter name sets.
	// Pushed while resolving a generic alias template or a generic
	// function's signature/body; a TypeName matching an in-scope entry
	// resolves to a KindTypeParam rather than an alias lookup.
	typeParamScopes []map[string]bool
}

// pushTypeParams makes `names` resolve as generic type variables for the
// duration of a matching popTypeParams. Empty lists are a no-op so callers
// can invoke it unconditionally.
func (c *checker) pushTypeParams(names []string) {
	if len(names) == 0 {
		return
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	c.typeParamScopes = append(c.typeParamScopes, set)
}

// popTypeParams unwinds a pushTypeParams. The `names` argument mirrors the
// push so an empty list stays a no-op and push/pop pair up symmetrically.
func (c *checker) popTypeParams(names []string) {
	if len(names) == 0 || len(c.typeParamScopes) == 0 {
		return
	}
	c.typeParamScopes = c.typeParamScopes[:len(c.typeParamScopes)-1]
}

// isTypeParam reports whether `name` is an in-scope generic type variable.
func (c *checker) isTypeParam(name string) bool {
	for i := len(c.typeParamScopes) - 1; i >= 0; i-- {
		if c.typeParamScopes[i][name] {
			return true
		}
	}
	return false
}

// expandRHS evaluates an explist of length m against an N-target slot list
// and returns N types. Lua's calling convention spreads the LAST expression
// across the remaining slots when it's multi-return-capable (call, method
// call, or vararg). Without that capability missing slots are nil.
//
// In gradual mode we don't track the exact number of returns a call
// produces, so trailing slots fed by a call get `any` (the call's result
// type might already be `any` or a single declared type — but its tail is
// unknown). Vararg also yields `any`.
func (c *checker) expandRHS(values []ast.Expression, n int) []*Type {
	out := make([]*Type, n)
	for i := 0; i < n; i++ {
		out[i] = nilT
	}
	if len(values) == 0 {
		return out
	}
	// First m-1 values map 1:1.
	m := len(values)
	for i := 0; i < m-1 && i < n; i++ {
		out[i] = c.typeOfExpression(values[i])
	}
	last := values[m-1]
	if m-1 < n {
		out[m-1] = c.typeOfExpression(last)
	} else {
		// Last value evaluated for nested errors but not used.
		c.walkExpressionDiscard(last)
	}
	// Spread tail when last is a multi-return producer.
	switch last.(type) {
	case *ast.CallExpression, *ast.MethodCallExpression, *ast.VarargExpression:
		for i := m; i < n; i++ {
			out[i] = anyT
		}
	}
	return out
}

// installGlobals seeds the env with the stdlib type signatures. The full
// signature surface lives in stdlib_types.go.
func (c *checker) installGlobals() {
	for name, t := range stdlibGlobals() {
		c.env.define(name, t)
	}
}

// preResolveAliases scans top-level statements for `type Name = T` and
// registers each in the alias table. Resolution happens in two passes so
// forward references work.
//
// Enum declarations contribute to the same alias table: `enum Color
// RED, GREEN end` registers `Color` as an alias for `number` (v1 — the
// stricter literal-union shape that would pin Color to the exact set
// {1, 2} is a follow-up). The number-alias is lossy in that any number
// satisfies a `: Color` annotation, but it lets typed function
// signatures (`function paint(c: Color)`) parse and compose correctly
// today, and the runtime still guarantees `Color.RED == 1` and that the
// table is frozen.
func (c *checker) preResolveAliases(stmts []ast.Statement) {
	// Pass 1: register placeholders so name → never (sentinel). Generic
	// aliases register an empty scheme carrying their parameter arity, so
	// forward references to them (`Other<T>` inside another template) can be
	// arity-checked in pass 2 before their own template is resolved.
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok {
			if len(a.TypeParams) > 0 {
				c.env.genericAliases[a.Name] = &GenericScheme{Params: a.TypeParams}
			} else {
				c.env.aliases[a.Name] = neverT
			}
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			c.env.aliases[e.Name.Name] = neverT
		}
	}
	// Pass 2: resolve each alias's RHS with the table populated. Self-
	// references at top level resolve to whatever they reference, with
	// recursive cycles yielding neverT (which produces a downstream
	// "unknown" error if used).
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok {
			if len(a.TypeParams) > 0 {
				c.pushTypeParams(a.TypeParams)
				c.env.genericAliases[a.Name].Template = c.resolveAST(a.Target)
				c.popTypeParams(a.TypeParams)
			} else {
				c.env.aliases[a.Name] = c.resolveAST(a.Target)
			}
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			c.env.aliases[e.Name.Name] = numberT
		}
	}
}

// ---------------------------------------------------------------------------
// Statement walker
// ---------------------------------------------------------------------------

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
		// Inside the loop body the condition held true, so its truthy
		// narrowing applies (e.g. `while x ~= nil do` makes x non-nil).
		c.applyRefinement(c.refine(n.Condition, true))
		c.walkBlock(n.Body)
		c.env.pop()
	case *ast.RepeatStatement:
		c.env.push()
		c.walkBlock(n.Body)
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
	case *ast.DeferStatement:
		// The deferred call is checked like any other call statement; it
		// produces no value the surrounding scope can observe.
		c.walkExpressionDiscard(n.Call)
	case *ast.ReturnStatement:
		c.walkReturn(n)
	case *ast.TypeAliasStatement, *ast.LabelStatement, *ast.BreakStatement,
		*ast.GotoStatement:
		// Type aliases were handled in the pre-pass; the others carry no
		// type-relevant state.
	case *ast.EnumStatement:
		// Pre-pass already registered Name as an alias. Bind the value-
		// side as `any` so member access (`Color.RED`) doesn't get
		// flagged as accessing fields of a non-structural type — the
		// runtime table is structurally `{[string]: number}`, but
		// pinning that precisely would force every user of the alias to
		// disambiguate value-vs-type at the use site. `any` is the
		// existing escape hatch the checker already uses for runtime
		// values it can't model statically.
		if n.Name != nil {
			c.env.define(n.Name.Name, anyT)
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
			bound = values[i]
		}
		c.env.define(name.Name, bound)
	}
}

func (c *checker) walkLocalFunctionStatement(s *ast.LocalFunctionStatement) {
	// Bind the name first so the body can reference itself recursively
	// with the declared signature visible.
	shape := c.functionShapeFromExpr(s.Func)
	c.env.define(s.Name, shape)
	c.walkFunctionBody(s.Func, shape.Fn)
}

func (c *checker) walkFunctionDeclaration(s *ast.FunctionDeclaration) {
	shape := c.functionShapeFromExpr(s.Func)
	if len(s.DottedFields) == 0 && s.MethodName == "" {
		// Plain `function name`: define as a global. We model globals via
		// the env's outermost frame (installed by installGlobals).
		c.env.define(s.Name.Name, shape)
	}
	// For dotted/method declarations the receiver is a runtime table; we
	// don't model deep table-method registration in v1.
	c.walkFunctionBody(s.Func, shape.Fn)
}

func (c *checker) walkAssignStatement(s *ast.AssignStatement) {
	rhs := c.expandRHS(s.Values, len(s.Targets))
	for i, t := range s.Targets {
		switch tgt := t.(type) {
		case *ast.Identifier:
			declared, ok := c.env.lookup(tgt.Name)
			if !ok {
				// First-time global write: bind to the RHS type so later
				// reads see it. Matches Lua's "globals materialize on
				// first assignment" model.
				c.env.define(tgt.Name, rhs[i])
				continue
			}
			if !assignable(rhs[i], declared) {
				c.errAssign(s.Line(), rhs[i], declared)
			}
		case *ast.IndexExpression:
			// Field assignment to a table. We currently don't enforce
			// shape conformance on writes (most Lua tables are open),
			// but we do want the base to look like a table or any.
			base := c.typeOfExpression(tgt.Object)
			if base.Kind != KindTable && base.Kind != KindAny {
				c.errf(s.Line(), "index-non-table",
					"cannot index a value of type %q", base.String())
			}
		}
	}
}

func (c *checker) walkIfStatement(s *ast.IfStatement) {
	// An accumulator frame carries the *negation* of every clause we've
	// already passed, so that an `elseif`/`else` is checked knowing each
	// earlier condition was false — exactly Lua's evaluation order. The
	// then-branch of each clause gets its own child frame on top.
	c.env.push()
	defer c.env.pop()

	for _, cl := range s.Clauses {
		// Walk the condition once (in the already-narrowed scope) for error
		// checking; refine() below is side-effect free and re-reads types.
		c.walkExpressionDiscard(cl.Condition)

		thenR := c.refine(cl.Condition, true)
		c.env.push()
		c.applyRefinement(thenR)
		c.walkBlock(cl.Body)
		c.env.pop()

		// Fold this clause's "condition is false" narrowing into the
		// accumulator so subsequent branches see it.
		c.applyRefinement(c.refine(cl.Condition, false))
	}

	if s.Else != nil {
		c.env.push()
		c.walkBlock(s.Else)
		c.env.pop()
	}
}

func (c *checker) walkNumericFor(s *ast.NumericForStatement) {
	// `for i = a, b [, c] do` — start/limit/step must be numbers.
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
	c.env.define(s.Name, numberT)
	c.walkBlock(s.Body)
	c.env.pop()
}

func (c *checker) walkGenericFor(s *ast.GenericForStatement) {
	for _, e := range s.Exprs {
		c.walkExpressionDiscard(e)
	}
	c.env.push()
	for _, name := range s.Names {
		// We don't model iterator-result types in v1; bind each name to
		// `any` so subsequent code in the loop body type-checks loosely.
		c.env.define(name, anyT)
	}
	c.walkBlock(s.Body)
	c.env.pop()
}

func (c *checker) walkReturn(r *ast.ReturnStatement) {
	if len(c.returnsStack) == 0 {
		// Top-level return — Lua allows `return` from a chunk; the chunk
		// has no declared return type, so we accept whatever.
		for _, v := range r.Values {
			c.walkExpressionDiscard(v)
		}
		return
	}
	declared := c.returnsStack[len(c.returnsStack)-1]
	if declared == nil {
		// Function had no return-type annotation — be permissive.
		for _, v := range r.Values {
			c.walkExpressionDiscard(v)
		}
		return
	}
	for i, v := range r.Values {
		got := c.typeOfExpression(v)
		if i >= len(declared) {
			// Returning more values than declared. Lua silently ignores
			// extras at the call site, but it's clearly a type-doc bug.
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
		// Missing return values surface as `nil` per Lua semantics; only
		// flag when the missing slot's declared type doesn't accept nil.
		for i := len(r.Values); i < len(declared); i++ {
			if !assignable(nilT, declared[i]) {
				c.errf(r.Line(), "missing-return",
					"function declared to return %d values, returning %d (slot %d expects %q)",
					len(declared), len(r.Values), i+1, declared[i].String())
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Function-body machinery
// ---------------------------------------------------------------------------

// functionShapeFromExpr builds a Type{KindFunction, Fn} from a
// FunctionExpression's annotations. Missing param types default to `any`
// (or are flagged in strict mode).
func (c *checker) functionShapeFromExpr(fe *ast.FunctionExpression) *Type {
	if fe == nil {
		return anyT
	}
	// Generic parameters are in scope while resolving the signature so `T`
	// in a param/return annotation becomes an opaque type variable.
	c.pushTypeParams(fe.TypeParams)
	defer c.popTypeParams(fe.TypeParams)
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

// walkFunctionBody pushes a fresh frame, binds params, walks the body, and
// pops. The return-stack tracks declared return types so ReturnStatement
// can check flow.
func (c *checker) walkFunctionBody(fe *ast.FunctionExpression, shape *FunctionShape) {
	if fe == nil || fe.Body == nil {
		return
	}
	// Keep the function's generic parameters in scope for the body so that
	// annotations inside it (`local acc: T`) resolve to the type variable.
	c.pushTypeParams(fe.TypeParams)
	defer c.popTypeParams(fe.TypeParams)
	c.env.push()
	defer c.env.pop()
	for i, p := range fe.Params {
		c.env.define(p.Name.Name, shape.Params[i])
	}
	// Push declared returns (nil if unannotated → permissive).
	var declared []*Type
	if len(shape.Returns) > 0 {
		declared = shape.Returns
	}
	c.returnsStack = append(c.returnsStack, declared)
	defer func() { c.returnsStack = c.returnsStack[:len(c.returnsStack)-1] }()
	c.walkBlock(fe.Body)
}

// ---------------------------------------------------------------------------
// Expression typing
// ---------------------------------------------------------------------------

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
		return booleanT
	case *ast.IntegerLiteral, *ast.FloatLiteral:
		return numberT
	case *ast.StringLiteral:
		return stringT
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
	}
	return anyT
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
	// Generic call: infer a concrete type for each type parameter from the
	// arguments, then check and return against the instantiated signature.
	if len(fn.TypeParams) > 0 {
		fn = c.instantiateCall(fn, args)
	}
	c.checkCallArgs(call.Line(), fn, args)
	if len(fn.Returns) == 0 {
		// Unannotated functions have no declared returns, but Lua callers
		// routinely use them in multi-value contexts. Returning `any`
		// (rather than `nil`) keeps gradual code unblocked.
		return anyT
	}
	return fn.Returns[0]
}

// instantiateCall infers type arguments for a generic function from the call
// arguments and returns a concrete (non-generic) FunctionShape with the
// inferred substitution applied to params, returns, and vararg type. Type
// params that inference can't pin down default to `any` — matching the
// gradual stance everywhere else in the checker.
func (c *checker) instantiateCall(fn *FunctionShape, args []*Type) *FunctionShape {
	subst := make(map[string]*Type, len(fn.TypeParams))
	// Positional params against provided args.
	n := len(args)
	if len(fn.Params) < n {
		n = len(fn.Params)
	}
	for i := 0; i < n; i++ {
		unify(fn.Params[i], args[i], subst)
	}
	// Trailing args flow into the vararg type, if any.
	if fn.IsVararg && fn.VarargType != nil && len(args) > len(fn.Params) {
		for _, a := range args[len(fn.Params):] {
			unify(fn.VarargType, a, subst)
		}
	}
	// Any param never mentioned by the arguments defaults to `any`.
	for _, p := range fn.TypeParams {
		if subst[p] == nil {
			subst[p] = anyT
		}
	}
	return &FunctionShape{
		Params:     substituteList(fn.Params, subst),
		Returns:    substituteList(fn.Returns, subst),
		IsVararg:   fn.IsVararg,
		VarargType: substituteType(fn.VarargType, subst),
		// TypeParams cleared: the returned shape is fully instantiated.
	}
}

// unify walks a declared parameter type alongside a concrete argument type,
// binding any type variables it encounters in `subst`. It is intentionally
// lightweight (no occurs-check, no full HM): it descends matching function
// and table structure and records leaf-level type-param bindings. When a
// variable is seen more than once with differing types the bindings are
// merged into a union, so `pair<T>(a: T, b: T)` called with (number, string)
// infers `T = number | string` rather than erroring.
func unify(param, arg *Type, subst map[string]*Type) {
	if param == nil || arg == nil {
		return
	}
	switch param.Kind {
	case KindTypeParam:
		if existing, ok := subst[param.TypeParam]; ok && existing != nil {
			if !Same(existing, arg) {
				subst[param.TypeParam] = NewUnion(existing, arg)
			}
			return
		}
		subst[param.TypeParam] = arg
	case KindFunction:
		// Nothing to learn from a non-function argument (e.g. `any`).
		af := callableShape(arg)
		if param.Fn == nil || af == nil {
			return
		}
		pn := len(param.Fn.Params)
		if len(af.Params) < pn {
			pn = len(af.Params)
		}
		for i := 0; i < pn; i++ {
			unify(param.Fn.Params[i], af.Params[i], subst)
		}
		rn := len(param.Fn.Returns)
		if len(af.Returns) < rn {
			rn = len(af.Returns)
		}
		for i := 0; i < rn; i++ {
			unify(param.Fn.Returns[i], af.Returns[i], subst)
		}
	case KindTable:
		if param.Table == nil || arg.Kind != KindTable || arg.Table == nil {
			return
		}
		if param.Table.Indexer != nil && arg.Table.Indexer != nil {
			unify(param.Table.Indexer.Key, arg.Table.Indexer.Key, subst)
			unify(param.Table.Indexer.Value, arg.Table.Indexer.Value, subst)
		}
		argFields := make(map[string]*Type, len(arg.Table.Fields))
		for _, f := range arg.Table.Fields {
			argFields[f.Key] = f.Type
		}
		for _, f := range param.Table.Fields {
			if at, ok := argFields[f.Key]; ok {
				unify(f.Type, at, subst)
			}
		}
	}
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
		bound := len(args)
		if bound > len(fn.Params) {
			bound = len(fn.Params)
		}
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

func (c *checker) typeOfBinary(e *ast.BinaryExpression) *Type {
	left := c.typeOfExpression(e.Left)
	right := c.typeOfExpression(e.Right)
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
		// `a and b` returns a if falsy, else b. The result type is the
		// union; for a typed-correctness perspective this is good enough.
		return NewUnion(left, right)
	case "or":
		return NewUnion(left, right)
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