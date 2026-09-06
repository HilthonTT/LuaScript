package bytecode

import (
	"fmt"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

type funcCtx struct {
	parent       *funcCtx
	is           *InstructionSet
	locals       *localTable
	upvals       []UpvalueDesc
	loops        []*loopFrame
	labels       map[string]labelInfo
	pendingGotos []pendingGoto
	pending      []*Instruction

	tryDepth int

	tryRegions []int

	// tbcDepth counts the to-be-closed (`local x <close>`) variables currently
	// in static scope; tbcScopes remembers the depth at each open block so the
	// matching CloseTBC can be emitted when the block ends.
	tbcDepth  int
	tbcScopes []int
}

type labelInfo struct {
	line       int
	tryRegions []int
}

type pendingGoto struct {
	label      string
	line       int
	anchor     *anchor
	tryRegions []int
}

type loopFrame struct {
	breakAnchor    *anchor
	continueAnchor *anchor

	isRepeat            bool
	repeatScopeIdx      int
	minContinueBindings int

	tryDepth int
	tbcDepth int
}

func (g *Generator) pushLoop(f *loopFrame) *loopFrame {
	f.tryDepth = g.current.tryDepth
	f.tbcDepth = g.current.tbcDepth
	g.current.loops = append(g.current.loops, f)
	return f
}

func (g *Generator) popLoop() {
	g.current.loops = g.current.loops[:len(g.current.loops)-1]
}

func (g *Generator) exitTryDepth(frame *loopFrame) int {
	return g.current.tryDepth - frame.tryDepth
}

// exitTBCDepth reports how many to-be-closed variables a break/continue out of
// frame has to close before jumping.
func (g *Generator) exitTBCDepth(frame *loopFrame) int {
	return g.current.tbcDepth - frame.tbcDepth
}

// openScope begins a block scope. Pair every call with closeScope so that any
// to-be-closed variable declared inside gets a CloseTBC on block exit.
func (g *Generator) openScope() {
	g.current.locals.openScope()
	g.current.tbcScopes = append(g.current.tbcScopes, g.current.tbcDepth)
}

func (g *Generator) closeScope(is *InstructionSet, line int) {
	if n := len(g.current.tbcScopes); n > 0 {
		base := g.current.tbcScopes[n-1]
		g.current.tbcScopes = g.current.tbcScopes[:n-1]
		if d := g.current.tbcDepth - base; d > 0 {
			is.define(CloseTBC, line, d)
			g.current.tbcDepth = base
		}
	}
	g.current.locals.closeScope()
}

type Generator struct {
	REPL    bool
	chunks  []*InstructionSet
	current *funcCtx
	errs    []error

	nextTryRegion int

	funcNames map[*ast.FunctionExpression]string
}

func (g *Generator) nameFunc(e ast.Expression, name string) {
	fe, ok := e.(*ast.FunctionExpression)
	if !ok || name == "" {
		return
	}
	if g.funcNames == nil {
		g.funcNames = map[*ast.FunctionExpression]string{}
	}
	g.funcNames[fe] = name
}

func (g *Generator) Err() error {
	if len(g.errs) == 0 {
		return nil
	}
	return g.errs[0]
}

func (g *Generator) checkPendingGotos(ctx *funcCtx) {
	for _, p := range ctx.pendingGotos {
		g.errs = append(g.errs, fmt.Errorf("line %d: no visible label '%s' for goto", p.line, p.label))
	}
	ctx.pendingGotos = nil
}

func NewGenerator() *Generator {
	return &Generator{}
}

func (g *Generator) ResetInstructionSets() { g.chunks = nil }

func (g *Generator) InitTopLevelScope(_ *ast.Program) {
	is := &InstructionSet{name: Program, isType: Program, IsVararg: true}
	g.current = &funcCtx{
		is:     is,
		locals: newLocalTable(nil),
		labels: map[string]labelInfo{},
	}
}

func (g *Generator) GenerateInstructions(stmts []ast.Statement) []*InstructionSet {
	g.compileStatements(stmts)
	g.endInstructions(g.current.is, lastLine(stmts))
	g.checkPendingGotos(g.current)

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

	out := append([]*InstructionSet{g.current.is}, g.chunks...)
	return out
}

func (g *Generator) allContexts() []*funcCtx {
	return []*funcCtx{g.current}
}

func (g *Generator) endInstructions(is *InstructionSet, sourceLine int) {
	if g.REPL && is.name == Program {
		return
	}
	is.define(Leave, sourceLine)
}

func (g *Generator) pushFunction(name string, params []ast.TypedParam, isVararg bool, sourceLine int) *funcCtx {
	is := &InstructionSet{name: name, isType: FunctionDef, IsVararg: isVararg, NumParams: len(params)}
	child := &funcCtx{
		parent: g.current,
		is:     is,
		locals: newLocalTable(g.current.locals),
		labels: map[string]labelInfo{},
	}
	for _, p := range params {
		child.locals.define(p.Name.Name)
	}
	parent := g.current
	g.current = child
	_ = sourceLine
	return parent
}

func (g *Generator) popFunction(parent *funcCtx, sourceLine int) int {
	g.endInstructions(g.current.is, sourceLine)
	g.checkPendingGotos(g.current)
	g.current.is.NumLocals = g.current.locals.maxSlot
	g.current.is.Upvalues = g.current.upvals

	parent.pending = append(parent.pending, g.current.pending...)

	parent.is.Protos = append(parent.is.Protos, g.current.is)
	idx := len(parent.is.Protos) - 1

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

func (ctx *funcCtx) recordPending(ins *Instruction) {
	ctx.pending = append(ctx.pending, ins)
}
