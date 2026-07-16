package vm

import "github.com/hilthontt/luascript/internal/compiler/bytecode"

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
	// Deferred holds the closures registered by `defer` in this activation.
	// They run in last-in-first-out order when the frame unwinds (see
	// VM.runDeferred). nil for the overwhelmingly common frame that defers
	// nothing, so the slice costs nothing until a `defer` actually fires.
	Deferred []*Closure
	// handlers holds the `try` regions currently open in this activation,
	// innermost last. Handlers live on the frame rather than on the VM so a
	// `return` out of a `try` needs no bookkeeping — unwinding the frame
	// discards them — and so a coroutine's handlers travel with its frames
	// across a yield/resume swap. nil until this activation runs a Try.
	handlers []tryHandler
}

// tryHandler is one open `try` region: where to resume, and the VM state to
// restore first. Everything here is captured when the Try opcode runs, i.e. at
// a statement boundary, so stackTop is the frame's clean stack height.
type tryHandler struct {
	catchIP   int // instruction index of the catch clause
	stackTop  int // len(vm.Stack) when Try ran
	markDepth int // len(vm.callMarks) when Try ran
}
