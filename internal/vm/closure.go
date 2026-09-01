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
	handlers []tryHandler
}

type tryHandler struct {
	catchIP   int
	stackTop  int
	markDepth int
}
