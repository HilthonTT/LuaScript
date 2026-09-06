package vm

import "github.com/hilthontt/luascript/internal/compiler/bytecode"

type Closure struct {
	Proto    *bytecode.InstructionSet
	Upvalues []*Upvalue
}

type GoFunc struct {
	Name string
	Fn   func(vm *VM, args []Value) []Value
}

type CallFrame struct {
	Closure  *Closure
	IP       int
	Base     int
	Top      int
	NResults int
	Varargs  []Value
	Deferred []*Closure
	// tbc holds the frame's live to-be-closed (`local x <close>`) values in
	// declaration order; CloseTBC pops them, and any left over are closed when
	// the frame goes away for any reason.
	tbc      []Value
	handlers []tryHandler
}

type tryHandler struct {
	catchIP   int
	stackTop  int
	markDepth int
	tbcTop    int
}
