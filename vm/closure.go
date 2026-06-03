package vm

import "github.com/hilthontt/luascript/compiler/bytecode"

// Closure is a Lua-defined function: a function prototype plus the upvalues
// captured at the moment the closure was created.
type Closure struct {
	Proto    *bytecode.InstructionSet
	Upvalues []*Upvalue
}

// GoFunc is a host-provided function callable from Lua. Args is read-only;
// the function returns its results as a slice (may be empty).
type GoFunc struct {
	Name string
	Fn   func(vm *VM, args []Value) []Value
}

// (Upvalue lives in upvalue.go.)

// CallFrame is one activation record. Base is the absolute index of the
// frame's first local slot on the operand stack; locals 0..NumLocals-1 live
// at Stack[Base..Base+NumLocals]. The returnTo state is captured by the
// caller frame; nresults says how many values the caller wants when the
// callee returns.
type CallFrame struct {
	Closure  *Closure
	IP       int
	Base     int // Stack index where this frame's locals begin
	Top      int // Stack index just past this frame's last allocated local
	NResults int // -1 means "all"
	Varargs  []Value
}
