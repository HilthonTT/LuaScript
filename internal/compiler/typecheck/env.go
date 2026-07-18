package typecheck

import "github.com/hilthontt/luascript/internal/compiler/ast"

// genericAlias is the template for a generic type alias or generic struct:
// its type parameters plus the unresolved target AST. Instantiation binds the
// parameters to concrete arguments and resolves the target (see
// resolveTypeApplication).
type genericAlias struct {
	params []string
	target ast.TypeNode
}

// env is the scoped type environment used while walking the AST. It mirrors
// the bytecode generator's localTable shape: a stack of frames where each
// frame holds a name → Type map. Lookups walk innermost-to-outermost.
//
// Type aliases live separately on the program-level `aliases` map because
// Luau (and v1) only allows them at chunk scope. They're populated in a
// pre-pass so forward references resolve.
type env struct {
	frames []frame

	// aliases is the program-wide alias table. The pre-pass owns
	// population; the walker only reads. Recursive aliases aren't in v1
	// (recursive references resolve to KindNever).
	aliases map[string]*Type

	// generics holds templates for parameterized aliases and structs
	// (`type Box<T> = ...`, `struct Box<T> { ... }`). A `Box<number>`
	// TypeApplication instantiates the template on demand.
	generics map[string]*genericAlias
}

type frame struct {
	bindings map[string]*Type

	// refined marks names whose binding in this frame is a narrowing shadow
	// installed by applyRefinement, not a declaration. Assignment checking
	// looks through these to the declared type (lookupDeclared), and an
	// assignment widens them in place (widenRefined) so a stale narrowing
	// can't outlive the value it described. Lazily allocated.
	refined map[string]bool
}

func newEnv() *env {
	return &env{
		frames:   []frame{{bindings: map[string]*Type{}}},
		aliases:  map[string]*Type{},
		generics: map[string]*genericAlias{},
	}
}

func (e *env) push() {
	e.frames = append(e.frames, frame{bindings: map[string]*Type{}})
}

func (e *env) pop() {
	if len(e.frames) == 0 {
		return
	}
	e.frames = e.frames[:len(e.frames)-1]
}

// define binds a name in the innermost frame, shadowing any outer binding.
// Re-defining within the same frame replaces the slot — matching Lua's
// `local x ... local x` shadowing semantics. A real declaration also clears
// any refinement mark left on the slot: `local s = ...` starts fresh.
func (e *env) define(name string, t *Type) {
	if len(e.frames) == 0 {
		return
	}
	f := &e.frames[len(e.frames)-1]
	f.bindings[name] = t
	delete(f.refined, name)
}

// defineRefined binds a narrowing shadow in the innermost frame. It types
// exactly like define for lookup, but is invisible to lookupDeclared and
// mutable by widenRefined.
func (e *env) defineRefined(name string, t *Type) {
	if len(e.frames) == 0 {
		return
	}
	f := &e.frames[len(e.frames)-1]
	f.bindings[name] = t
	if f.refined == nil {
		f.refined = map[string]bool{}
	}
	f.refined[name] = true
}

// lookupDeclared returns the innermost binding that is a declaration,
// seeing through refinement shadows. Assignments are checked against this —
// `s = nil` inside `if s ~= nil then` is legal because the *declared* type
// is `string?`, whatever the branch narrowed `s` to.
func (e *env) lookupDeclared(name string) (*Type, bool) {
	for i := len(e.frames) - 1; i >= 0; i-- {
		f := e.frames[i]
		if t, ok := f.bindings[name]; ok {
			if f.refined[name] {
				continue
			}
			return t, true
		}
	}
	return e.lookup(name)
}

// widenRefined folds an assigned type into every refinement shadow of
// `name`, so a narrowing can't keep claiming the pre-assignment type after
// `s = nil`. Widening with a union (rather than replacing) keeps outer
// shadows sound when the assignment sits in a deeper branch that may not
// execute on every path that reaches them.
func (e *env) widenRefined(name string, t *Type) {
	for i := range e.frames {
		f := &e.frames[i]
		if f.refined[name] {
			f.bindings[name] = NewUnion(f.bindings[name], t)
		}
	}
}

// lookup walks innermost-to-outermost. Returns the bound type plus true on
// hit; nil/false on miss (callers fall back to globals → any).
func (e *env) lookup(name string) (*Type, bool) {
	for i := len(e.frames) - 1; i >= 0; i-- {
		if t, ok := e.frames[i].bindings[name]; ok {
			return t, true
		}
	}
	return nil, false
}

// alias resolves a user-named type. Unknown names return KindNever (which
// flows to everything but never satisfies a non-never slot, surfacing as
// an error at the use site rather than crashing the walker).
func (e *env) alias(name string) *Type {
	if t, ok := e.aliases[name]; ok {
		return t
	}
	return neverT
}
