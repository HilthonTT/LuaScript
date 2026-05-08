package typecheck

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
}

type frame struct {
	bindings map[string]*Type
}

func newEnv() *env {
	return &env{
		frames:  []frame{{bindings: map[string]*Type{}}},
		aliases: map[string]*Type{},
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
// `local x ... local x` shadowing semantics.
func (e *env) define(name string, t *Type) {
	if len(e.frames) == 0 {
		return
	}
	e.frames[len(e.frames)-1].bindings[name] = t
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
