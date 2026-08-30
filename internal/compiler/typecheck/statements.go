package typecheck

// The statement walker.

import "github.com/hilthontt/luascript/internal/compiler/ast"

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
		c.recordEnum(n)
		if n.IsTagged() {
			// Tagged enum: bind the namespace value to a table typing each
			// variant. Payload variants are constructors `(P...) -> Enum`;
			// nullary variants are `Enum` singletons. This lets the checker
			// validate `Shape.Circle(5)` arity/args and yield a `Shape`.
			c.env.define(n.Name.Name, c.taggedEnumNamespaceType(n))
			break
		}
		// Classic integer enum: bind the value-side to a table typing each
		// member as the singleton number it is. `Color.RED` is therefore the
		// literal `1`, which both satisfies a `: Color` slot and acts as a
		// discriminator in `if c == Color.RED then`. No indexer is declared,
		// so a misspelled `Color.REDD` is an error while a dynamic
		// `Color[name]` still falls back to `any`.
		c.env.define(n.Name.Name, classicEnumNamespaceType(n))
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
			// No annotation: widen, so `local mode = "read"` is a `string`
			// the programmer can reassign rather than a singleton that
			// rejects every later value.
			bound = widen(values[i])
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
				// first assignment" model. Widened for the same reason an
				// un-annotated local is.
				c.env.define(tgt.Name, widen(rhs[i]))
				continue
			}
			if !assignable(rhs[i], declared) {
				c.errAssign(s.Line(), rhs[i], declared)
			}
			// The value changed; any active narrowing must absorb the new
			// type or it would keep vouching for the old one.
			c.env.widenRefined(tgt.Name, widen(rhs[i]))
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
//
// When the subject's type has a finite domain (a tagged enum, a singleton
// union, a boolean) the arms are also checked for exhaustiveness — see
// exhaustive.go.
func (c *checker) walkMatchStatement(s *ast.MatchStatement) {
	subject := c.typeOfExpression(s.Subject)
	c.checkMatchExhaustive(s, subject)

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
