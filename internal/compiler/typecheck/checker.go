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
	c := &checker{
		env:          newEnv(),
		opts:         opts,
		taggedEnums:  map[string][]string{},
		classicEnums: map[string][]string{},
	}
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

	// taggedEnums / classicEnums record each declared enum's variant names in
	// source order, keyed by enum name. The *types* an enum contributes
	// (a nominal table, or a literal union) deliberately don't carry the
	// variant list; exhaustiveness checking is the one consumer that needs
	// it, so it lives here rather than widening Type. See exhaustive.go.
	taggedEnums  map[string][]string
	classicEnums map[string][]string

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
