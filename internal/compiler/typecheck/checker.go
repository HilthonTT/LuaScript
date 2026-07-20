package typecheck

import (
	"github.com/hilthontt/luascript/internal/compiler/ast"
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
	c.scanMutations(prog.Block)
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

	// instDepth counts nested generic instantiations. Self-referential
	// generics (`type L<T> = { next: L<T> }`) would otherwise expand
	// forever and blow the Go stack — a fatal, unrecoverable crash.
	instDepth int

	// builtins pins the exact stdlib signatures installGlobals bound for the
	// names narrowing trusts (assert/error/type/typeof). builtinInScope
	// compares against these so a shadowed builtin stops driving narrowing.
	builtins map[string]*Type

	// assignedSomewhere / upvalMutated are filled by scanMutations before
	// the walk: every name assigned anywhere in the program, and the subset
	// assigned by some function literal as an upvalue. See refine.go.
	assignedSomewhere map[string]bool
	upvalMutated      map[string]bool
}

// builtinInScope reports whether `name` still denotes the stdlib builtin at
// this point — i.e. it resolves to the exact signature installGlobals bound.
// Re-aliasing the real builtin (`local assert = assert`) keeps the same
// signature value and stays trusted; any other shadowing does not.
func (c *checker) builtinInScope(name string) bool {
	t, ok := c.env.lookup(name)
	return ok && t == c.builtins[name]
}

// invalidateCallRefinements widens every visible refinement of a variable
// some closure mutates as an upvalue: an arbitrary call may reach that
// closure, so the narrowing can't be trusted past it.
func (c *checker) invalidateCallRefinements() {
	if len(c.upvalMutated) == 0 {
		return
	}
	for name := range c.upvalMutated {
		if !c.env.visiblyRefined(name) {
			continue
		}
		if declared, ok := c.env.lookupDeclared(name); ok {
			c.env.widenRefined(name, declared)
		}
	}
}

// widenLoopAssigned widens refinements established *outside* a loop for
// every name the loop body assigns: on the second iteration those statements
// re-execute after the assignment, so a pre-loop narrowing can't vouch for
// them. Called after pushing the loop frame and before applying the loop
// condition's own refinement (which is re-established every iteration and
// therefore stays sound).
func (c *checker) widenLoopAssigned(b *ast.Block) {
	if b == nil {
		return
	}
	assigned := map[string]bool{}
	upval := map[string]bool{}
	scanBlockMutations(b, assigned, upval, false, nil)
	for name := range assigned {
		if !c.env.visiblyRefined(name) {
			continue
		}
		if declared, ok := c.env.lookupDeclared(name); ok {
			c.env.widenRefined(name, declared)
		}
	}
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
	for i := range n {
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
	g := stdlibGlobals()
	for name, t := range g {
		c.env.define(name, t)
	}
	c.builtins = map[string]*Type{
		"assert": g["assert"],
		"error":  g["error"],
		"type":   g["type"],
		"typeof": g["typeof"],
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
	// declarations register their template immediately (instantiation is
	// lazy, so no forward-reference problem) and skip the placeholder.
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok {
			if len(a.TypeParams) > 0 {
				c.env.generics[a.Name] = &genericAlias{params: a.TypeParams, target: a.Target}
			} else {
				c.env.aliases[a.Name] = neverT
			}
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			c.env.aliases[e.Name.Name] = neverT
		}
		if st, ok := s.(*ast.StructStatement); ok && st.Name != nil {
			if len(st.TypeParams) > 0 {
				c.env.generics[st.Name.Name] = &genericAlias{
					params: st.TypeParams,
					target: structTableAST(st),
				}
			} else {
				c.env.aliases[st.Name.Name] = neverT
			}
		}
	}
	// Pass 2: resolve each alias's RHS with the table populated. Self-
	// references at top level resolve to whatever they reference, with
	// recursive cycles yielding neverT (which produces a downstream
	// "unknown" error if used).
	for _, s := range stmts {
		if a, ok := s.(*ast.TypeAliasStatement); ok && len(a.TypeParams) == 0 {
			c.env.aliases[a.Name] = c.resolveAST(a.Target)
		}
		if e, ok := s.(*ast.EnumStatement); ok && e.Name != nil {
			if e.IsTagged() {
				// A tagged enum's *type* is a nominal opaque table: field/
				// index access on a value yields `any` (so `s.__tag`, `s[1]`
				// don't error), but a bare number/string is NOT a Shape.
				t := NewTable(nil, &Indexer{Key: anyT, Value: anyT})
				t.AliasName = e.Name.Name
				c.env.aliases[e.Name.Name] = t
			} else {
				c.env.aliases[e.Name.Name] = numberT
			}
		}
		if st, ok := s.(*ast.StructStatement); ok && st.Name != nil && len(st.TypeParams) == 0 {
			// A struct name resolves, as a *type*, to its structural table
			// `{ f1: T1, ... }` tagged with the struct's name for readable
			// diagnostics. The *value* side (the constructor) is bound later
			// in walkStatement. Generic structs live in the generics table
			// instead and are instantiated via `Name<Args>`.
			c.env.aliases[st.Name.Name] = c.structType(st)
		}
	}
}

// structTableAST builds a synthetic table-type AST for a struct's fields, so
// a generic struct can be stored as a generic alias template and instantiated
// through the same `Name<Args>` path as a generic `type` alias.
func structTableAST(s *ast.StructStatement) ast.TypeNode {
	fields := make([]ast.TypeTableField, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = ast.TypeTableField{Key: f.Name, Value: f.Type}
	}
	return &ast.TypeTable{BaseNode: s.BaseNode, Fields: fields}
}

// structType builds the nominal table type a struct name resolves to. The
// AliasName is the struct name so `typeof(p)`-style diagnostics show `Point`
// rather than the expanded `{ x: number, y: number }`.
func (c *checker) structType(s *ast.StructStatement) *Type {
	fields := make([]TableField, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = TableField{Key: f.Name, Type: c.resolveAST(f.Type)}
	}
	t := NewTable(fields, nil)
	t.AliasName = s.Name.Name
	return t
}

// taggedEnumNamespaceType builds the type of a tagged enum's namespace
// value. Each variant becomes a field: payload variants map to constructor
// functions `(P...) -> Enum`, nullary variants map to `Enum` itself.
func (c *checker) taggedEnumNamespaceType(e *ast.EnumStatement) *Type {
	enumT := c.env.alias(e.Name.Name) // the nominal type registered in the pre-pass
	fields := make([]TableField, 0, len(e.Variants))
	for _, v := range e.Variants {
		if len(v.Payload) == 0 {
			fields = append(fields, TableField{Key: v.Name, Type: enumT})
			continue
		}
		params := make([]*Type, len(v.Payload))
		for i, p := range v.Payload {
			params[i] = c.resolveAST(p)
		}
		ctor := NewFunction(params, []*Type{enumT}, false, nil)
		fields = append(fields, TableField{Key: v.Name, Type: ctor})
	}
	return NewTable(fields, nil)
}

// structConstructorType builds the constructor function type bound to a
// struct's value name. Positionally it is `(T1, T2, ...) -> Struct`; the
// Struct marker also lets typeOfCall accept the `Name{ ... }` named form.
func (c *checker) structConstructorType(s *ast.StructStatement) *Type {
	// For a generic struct the field types mention the parameters, so bring
	// them into scope as type variables while building the constructor. A
	// call like `Box(5)` then infers `T` and yields `Box<number>`.
	restore := c.pushTypeParams(s.TypeParams)
	defer restore()

	shape := c.structType(s)
	params := make([]*Type, len(shape.Table.Fields))
	for i, f := range shape.Table.Fields {
		params[i] = f.Type
	}
	return &Type{
		Kind: KindFunction,
		Fn: &FunctionShape{
			Params:     params,
			Returns:    []*Type{shape},
			TypeParams: s.TypeParams,
			Struct:     &StructCtor{Name: s.Name.Name, Shape: shape.Table},
		},
	}
}

// Statement walker

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
		// Refinements from outside the loop can't vouch for variables the
		// body assigns — iteration 2 re-runs the body after the assignment.
		c.widenLoopAssigned(n.Body)
		// Inside the loop body the condition held true, so its truthy
		// narrowing applies (e.g. `while x ~= nil do` makes x non-nil).
		// This is re-established every iteration, so it goes on top of the
		// loop-assignment widening.
		c.applyRefinement(c.refine(n.Condition, true))
		c.walkBlock(n.Body)
		c.env.pop()
	case *ast.RepeatStatement:
		c.env.push()
		c.widenLoopAssigned(n.Body)
		c.walkBlock(n.Body)
		if blockHasDirectContinue(n.Body) {
			// A continue jumps straight to the `until` condition, bypassing
			// whatever narrowing the fall-through path established in the
			// body frame — drop those before checking the condition. (The
			// body's locals themselves stay in scope, as at runtime.)
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
		// `assert(cond)` only returns when cond held, so its positive
		// narrowing applies to the rest of the block — but only while
		// `assert` still denotes the builtin; a shadowing definition makes
		// the call prove nothing.
		if cond, ok := assertCondition(n.Expression); ok && c.builtinInScope("assert") {
			c.applyRefinement(c.refine(cond, true))
		}
	case *ast.DeferStatement:
		// The deferred call is checked like any other call statement; it
		// produces no value the surrounding scope can observe.
		c.walkExpressionDiscard(n.Call)
	case *ast.MatchStatement:
		c.walkMatchStatement(n)
	case *ast.TryCatchStatement:
		c.env.push()
		c.walkBlock(n.Try)
		c.env.pop()
		c.env.push()
		if n.CatchVar != nil {
			// Anything can be thrown — a string, a table, a number — and v1
			// has no way to narrow that, so the binding is `any`.
			c.env.define(n.CatchVar.Name, anyT)
		}
		c.walkBlock(n.Catch)
		c.env.pop()
	case *ast.ThrowStatement:
		// Like `error(v)`, any value may be thrown; nothing to constrain.
		c.walkExpressionDiscard(n.Value)
	case *ast.ReturnStatement:
		c.walkReturn(n)
	case *ast.TypeAliasStatement, *ast.LabelStatement, *ast.BreakStatement,
		*ast.ContinueStatement, *ast.GotoStatement:
		// Type aliases were handled in the pre-pass; the others carry no
		// type-relevant state.
	case *ast.EnumStatement:
		if n.Name == nil {
			break
		}
		if n.IsTagged() {
			// Tagged enum: bind the namespace value to a table typing each
			// variant. Payload variants are constructors `(P...) -> Enum`;
			// nullary variants are `Enum` singletons. This lets the checker
			// validate `Shape.Circle(5)` arity/args and yield a `Shape`.
			c.env.define(n.Name.Name, c.taggedEnumNamespaceType(n))
			break
		}
		// Classic integer enum: bind the value-side as `any` so member
		// access (`Color.RED`) doesn't get flagged as accessing fields of a
		// non-structural type. The runtime table is structurally
		// `{[string]: number}`, but pinning that precisely would force every
		// user of the alias to disambiguate value-vs-type at the use site.
		c.env.define(n.Name.Name, anyT)
	case *ast.StructStatement:
		// Pre-pass already registered Name as a type alias. Bind the value
		// side to the constructor function so `Point(1, 2)` / `Point{...}`
		// type-check and yield a `Point`.
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
			bound = values[i]
			if bound.Kind == KindNil {
				// `local f` / `local f = nil` without an annotation: the
				// forward-declaration idiom. Pinning the literal nil type
				// would reject every later assignment; untyped slots are
				// `any` by design.
				bound = anyT
			}
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
			// Check against the declared type, not any narrowing shadow —
			// `s = nil` inside `if s ~= nil then` is legal for a `string?`.
			declared, ok := c.env.lookupDeclared(tgt.Name)
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
			// The value changed; any active narrowing must absorb the new
			// type or it would keep vouching for the old one.
			c.env.widenRefined(tgt.Name, rhs[i])
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

// walkMatchStatement checks a `match`. Arms are independent: each gets its
// own scope holding that arm's binders, so a binder can never leak into a
// sibling arm or past the `end`. Nothing narrows the *subject* — patterns
// test a value the checker cannot re-associate with the scrutinee expression
// unless it is a simple name, and v1 does not track that.
func (c *checker) walkMatchStatement(s *ast.MatchStatement) {
	c.walkExpressionDiscard(s.Subject)

	for i := range s.Arms {
		arm := &s.Arms[i]

		// Value-pattern alternatives are compared with `==` against the
		// subject; walk them for their own errors.
		for _, v := range arm.Pattern.Values {
			c.walkExpressionDiscard(v)
		}

		c.env.push()

		// A typed pattern is the one form that proves something about what it
		// binds, so its binder gets the declared type; every other binder is
		// a projection out of a value of unknown shape.
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
			// The body only runs when the guard held.
			c.applyRefinement(c.refine(arm.Guard, true))
		}
		c.walkStatement(arm.Body)

		c.env.pop()
	}
}

func (c *checker) walkIfStatement(s *ast.IfStatement) {
	// An accumulator frame carries the *negation* of every clause we've
	// already passed, so that an `elseif`/`else` is checked knowing each
	// earlier condition was false — exactly Lua's evaluation order. The
	// then-branch of each clause gets its own child frame on top.
	c.env.push()

	// Early-exit narrowing: when a leading prefix of clauses always
	// terminates (`if s == nil then return end`), falling past the whole
	// statement proves each of those conditions was false, so their
	// negations outlive the `end`. Only a prefix qualifies — once a clause
	// can fall through, later conditions may never have been evaluated.
	var persist []refinement
	prefixTerminates := true

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

// Function-body machinery

// functionShapeFromExpr builds a Type{KindFunction, Fn} from a
// FunctionExpression's annotations. Missing param types default to `any`
// (or are flagged in strict mode).
func (c *checker) functionShapeFromExpr(fe *ast.FunctionExpression) *Type {
	if fe == nil {
		return anyT
	}
	// Generic parameters are in scope for the whole signature: register them
	// as gradual type variables while resolving param/return annotations.
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
			// The default must fit the declared type, and a defaulted
			// parameter is optional at every call site: callers may omit it
			// or pass nil, so the *signature* type is widened with nil.
			// Inside the body the prologue has already applied the default,
			// so walkFunctionBody binds the un-widened declared type.
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

// walkFunctionBody pushes a fresh frame, binds params, walks the body, and
// pops. The return-stack tracks declared return types so ReturnStatement
// can check flow.
func (c *checker) walkFunctionBody(fe *ast.FunctionExpression, shape *FunctionShape) {
	if fe == nil || fe.Body == nil {
		return
	}
	// Type parameters are in scope inside the body too (`local y: T = ...`).
	restore := c.pushTypeParams(fe.TypeParams)
	defer restore()

	c.env.push()
	defer c.env.pop()
	// A closure body must not trust a refinement of a captured variable that
	// can be reassigned: the closure may run after the mutation. Re-bind such
	// names to their declared types for the duration of the body walk.
	// Variables never assigned anywhere keep their narrowing (the common
	// `if x ~= nil then use(function() return x end) end` idiom).
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
			// The signature widened this parameter with nil for callers,
			// but the prologue guarantees the default has been applied by
			// the time the body runs — bind the declared type.
			bound = c.resolveAST(p.Type)
		}
		c.env.define(p.Name.Name, bound)
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
