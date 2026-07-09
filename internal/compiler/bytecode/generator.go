package bytecode

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

// funcCtx is the per-function emission state. The main chunk is itself a
// vararg function with no parameters.
type funcCtx struct {
	parent       *funcCtx
	is           *InstructionSet
	locals       *localTable
	upvals       []UpvalueDesc
	loops        []*loopFrame // active loops, innermost last
	labels       map[string]int
	pendingGotos []pendingGoto
	pending      []*Instruction // instructions with a *anchor parameter
}

type pendingGoto struct {
	label  string
	line   int
	anchor *anchor
}

// loopFrame collects the break and continue targets for a single active
// loop. breakAnchor resolves past the whole loop; continueAnchor resolves to
// the loop's "next iteration" point — the spot right after the body where
// per-iteration upvalue closing and the condition/step re-check happen.
type loopFrame struct {
	breakAnchor    *anchor
	continueAnchor *anchor

	// Repeat-only bookkeeping (isRepeat is false for the other loop forms).
	// A `continue` in repeat jumps straight to the `until` condition, which
	// is evaluated in the scope of body locals — so a local declared after
	// the continue is in scope there but its initialization was skipped on
	// that iteration, and its slot may hold a stale internal temporary.
	// Like Luau, we reject such programs at compile time (see
	// compileRepeat). repeatScopeIdx is the index of the repeat body's
	// scope frame; minContinueBindings is the fewest bindings that scope
	// held at any `continue` site (-1 = no continue seen).
	isRepeat            bool
	repeatScopeIdx      int
	minContinueBindings int
}

// Generator drives bytecode emission for an entire program.
type Generator struct {
	REPL    bool
	chunks  []*InstructionSet // every emitted instruction set, main chunk first
	current *funcCtx
	errs    []error // generation errors (e.g. goto to an undefined label)
}

// Err returns the first generation error, or nil. An unresolved forward
// goto would otherwise keep its anchor at line 0 and silently compile into
// a jump to the start of the function.
func (g *Generator) Err() error {
	if len(g.errs) == 0 {
		return nil
	}
	return g.errs[0]
}

// checkPendingGotos records an error for every goto in ctx whose label never
// appeared. Called when a function (or the main chunk) finishes emitting.
func (g *Generator) checkPendingGotos(ctx *funcCtx) {
	for _, p := range ctx.pendingGotos {
		g.errs = append(g.errs, fmt.Errorf("line %d: no visible label '%s' for goto", p.line, p.label))
	}
	ctx.pendingGotos = nil
}

// NewGenerator creates a fresh generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// ResetInstructionSets clears prior emission output. Useful between REPL inputs.
func (g *Generator) ResetInstructionSets() { g.chunks = nil }

// InitTopLevelScope opens the main-chunk function context. The Lua reference
// implementation models a chunk as `function(...) <chunk body> end`, so the
// main chunk is vararg with zero declared parameters.
func (g *Generator) InitTopLevelScope(_ *ast.Program) {
	is := &InstructionSet{name: Program, isType: Program, IsVararg: true}
	g.current = &funcCtx{
		is:     is,
		locals: newLocalTable(nil),
		labels: map[string]int{},
	}
}

// GenerateInstructions emits bytecode for the supplied chunk body and returns
// every instruction set produced (main chunk first, nested function protos
// follow in declaration order).
func (g *Generator) GenerateInstructions(stmts []ast.Statement) []*InstructionSet {
	g.compileStatements(stmts)
	g.endInstructions(g.current.is, lastLine(stmts))
	g.checkPendingGotos(g.current)

	// Resolve every forward-jump anchor: every *anchor stored in any
	// param position is replaced with its final target line, both in the
	// Params slice (for disassembly / Inspect) and in the typed A/B fast-
	// path fields that the VM hot loop reads. For-family opcodes carry
	// the anchor in the *second* slot (after baseSlot); everything else
	// carries it first — slot index drives whether the resolved value
	// lands in A or B.
	for _, ctx := range g.allContexts() {
		for _, ins := range ctx.pending {
			for i, p := range ins.Params {
				a, ok := p.(*anchor)
				if !ok {
					continue
				}
				ins.Params[i] = a.line
				switch i {
				case 0:
					ins.A = int32(a.line)
				case 1:
					ins.B = int32(a.line)
				}
			}
		}
	}

	// Splice main chunk to the front of the result.
	out := append([]*InstructionSet{g.current.is}, g.chunks...)
	return out
}

// allContexts walks the chunk + every nested function to collect all
// pending-anchor lists. We accumulate these on each ctx as it finishes.
func (g *Generator) allContexts() []*funcCtx {
	// In the current implementation, every nested function pushes its
	// pending list into the parent on close (see closeFunction), so
	// g.current holds the union. Returning [g.current] is enough.
	return []*funcCtx{g.current}
}

// endInstructions appends a Leave (frame terminator) unless this is the REPL
// main chunk, which is left dangling so the REPL can splice further input.
func (g *Generator) endInstructions(is *InstructionSet, sourceLine int) {
	if g.REPL && is.name == Program {
		return
	}
	is.define(Leave, sourceLine)
}

// pushFunction starts emitting a nested function's body. Returns the parent
// context so the caller can restore it after compilation. Type annotations
// on params are ignored here — the bytecode is type-erased; the type
// checker reads annotations from the AST directly before this pass runs.
func (g *Generator) pushFunction(name string, params []ast.TypedParam, isVararg bool, sourceLine int) *funcCtx {
	is := &InstructionSet{name: name, isType: FunctionDef, IsVararg: isVararg, NumParams: len(params)}
	child := &funcCtx{
		parent: g.current,
		is:     is,
		locals: newLocalTable(g.current.locals),
		labels: map[string]int{},
	}
	// Declare each parameter as a local in the new function's root scope.
	for _, p := range params {
		child.locals.define(p.Name.Name)
	}
	parent := g.current
	g.current = child
	_ = sourceLine
	return parent
}

// popFunction finalizes the current function, attaches its upvalue table and
// local-count, registers it on the parent's Protos, and returns its index.
func (g *Generator) popFunction(parent *funcCtx, sourceLine int) int {
	g.endInstructions(g.current.is, sourceLine)
	g.checkPendingGotos(g.current)
	g.current.is.NumLocals = g.current.locals.maxSlot
	g.current.is.Upvalues = g.current.upvals

	// Splice pending-anchor list into parent so the post-compile pass sees it.
	parent.pending = append(parent.pending, g.current.pending...)

	// Register as a nested proto on the parent's instruction set.
	parent.is.Protos = append(parent.is.Protos, g.current.is)
	idx := len(parent.is.Protos) - 1

	// Also keep a flat record for callers that want the full chunk list.
	g.chunks = append(g.chunks, g.current.is)

	g.current = parent
	return idx
}

func lastLine(stmts []ast.Statement) int {
	if len(stmts) == 0 {
		return 0
	}
	return stmts[len(stmts)-1].Line()
}

// recordPending registers an instruction whose first parameter is an anchor;
// it will be resolved to a concrete line after emission completes.
func (ctx *funcCtx) recordPending(ins *Instruction) {
	ctx.pending = append(ctx.pending, ins)
}
