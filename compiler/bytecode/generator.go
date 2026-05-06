package bytecode

import "github.com/hilthontt/sakura-lang/compiler/ast"

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
	anchor *anchor
}

// loopFrame collects break-target patches and continue-target line for a
// single active loop. Lua has no `continue`, so we only track break.
type loopFrame struct {
	breakAnchor *anchor
}

// Generator drives bytecode emission for an entire program.
type Generator struct {
	REPL    bool
	chunks  []*InstructionSet // every emitted instruction set, main chunk first
	current *funcCtx
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

	// Resolve every forward-jump anchor: every *anchor stored in any param
	// position is replaced with its final target line.
	for _, ctx := range g.allContexts() {
		for _, ins := range ctx.pending {
			for i, p := range ins.Params {
				if a, ok := p.(*anchor); ok {
					ins.Params[i] = a.line
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
// context so the caller can restore it after compilation.
func (g *Generator) pushFunction(name string, params []*ast.Identifier, isVararg bool, sourceLine int) *funcCtx {
	is := &InstructionSet{name: name, isType: FunctionDef, IsVararg: isVararg, NumParams: len(params)}
	child := &funcCtx{
		parent: g.current,
		is:     is,
		locals: newLocalTable(g.current.locals),
		labels: map[string]int{},
	}
	// Declare each parameter as a local in the new function's root scope.
	for _, p := range params {
		child.locals.define(p.Name)
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
