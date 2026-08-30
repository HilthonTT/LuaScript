package vm

import (
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// VM is the top-level interpreter state. It owns the operand stack, the
// frame stack, the globals table, and the open-upvalue list.
//
// When coroutines are in use, the VM's live Stack/frames/openUpvs always
// belong to the *currently executing* coroutine (or the main thread). On
// every resume/yield boundary the outgoing thread's state is saved into its
// Thread snapshot and the incoming thread's snapshot is loaded; see
// vm/coroutine.go.
type VM struct {
	Stack   []Value
	Globals *Table

	// stringMeta is the shared metatable for string values (Lua 5.4 §2.4
	// "the string library sets the metatable for strings"). The standard
	// pattern is to install `string` itself here so `("hi"):upper()` resolves.
	stringMeta *Table

	// mainThread is the snapshot of the program's top-level thread; used by
	// coroutine.resume to swap state back when a coroutine yields.
	mainThread *Thread
	// currentCo is the actively-running coroutine, or nil when running on
	// the main thread. coroutine.yield consults this to know which channels
	// to use, and to refuse yields from outside any coroutine.
	currentCo *Coroutine

	frames   []*CallFrame
	openUpvs []*Upvalue // sorted ascending by Index; head of the open-upvalue chain

	// callMarks tracks variadic-call argument bases. compileCall emits a
	// MarkArgs opcode before pushing args when the last argument is a
	// multi-value producer (call/methodcall/vararg). The matching Call
	// opcode (encoded with nargs=-1) pops the latest mark to learn the
	// args' starting slot, since the spread width isn't known statically.
	callMarks []int

	// framePool recycles CallFrame structs. Frames are pushed here by
	// unwindFrame once they are fully dead (popped off v.frames, stack
	// truncated) and handed back out by callClosure. Recycling is safe
	// across coroutines: a yielded coroutine keeps its *live* frames in its
	// Thread snapshot and never returns them here — only unwound frames are
	// pooled. This trims one heap allocation per call.
	framePool []*CallFrame

	// retScratch is a reused buffer for ferrying a frame's return values
	// across unwindFrame (which truncates the stack and re-appends over the
	// vacated region). Single-threaded VM execution means one return is
	// fully processed before the next, so a single shared buffer is safe.
	retScratch []Value

	mode parser.Mode
}

// New creates a fresh VM with an empty globals table.
func New() *VM {
	v := &VM{
		// 2048 entries (~16KB on 64-bit) gives enough headroom that deep
		// recursion and large local frames don't trigger backing-array
		// reallocations during the run. Per-VM cost is negligible.
		Stack:      make([]Value, 0, 2048),
		Globals:    NewTable(0, 32),
		mainThread: &Thread{},
		mode:       parser.NormalMode,
	}
	registerStdlib(v)
	registerCoroutineLibrary(v)
	registerLibraryModules(v)
	registerLoader(v)
	return v
}

// Run loads `main` as the top-level chunk and executes it. The chunk runs
// with no arguments and discards results. Any runtime error surfaces as a
// non-nil error.
func (v *VM) Run(main *bytecode.InstructionSet) (err error) {
	defer v.recoverToError(&err)

	cl := &Closure{Proto: main}
	v.callClosure(cl, nil, 0)
	return nil
}

// RunMainChunkWithResults is like Run but keeps every value the chunk
// returned and hands them back to the caller. The REPL uses it to print
// the value of bare expressions like Lua's interactive `lua` does.
func (v *VM) RunMainChunkWithResults(main *bytecode.InstructionSet) (results []Value, err error) {
	defer v.recoverToError(&err)

	base := len(v.Stack)
	cl := &Closure{Proto: main}
	v.callClosure(cl, nil, -1)
	results = append([]Value(nil), v.Stack[base:]...)
	v.Stack = v.Stack[:base]
	return results, nil
}

// recoverToError is the shared `defer`-installed recover for the top-level Run
// paths — the last boundary an error can reach. Nothing between the raise and
// here truncates v.frames (execCatching re-panics without unwinding when it
// has no handler), so the Lua call stack is still intact and toRuntimeError
// can snapshot it for the traceback.
//
// The result is always a *RuntimeError, whose Error() carries the positioned
// message plus the traceback. A Go panic that is not a Lua error at all still
// gets wrapped, so an internal fault is reported with the script location that
// triggered it rather than as a bare "vm panic".
func (v *VM) recoverToError(err *error) {
	if r := recover(); r != nil {
		*err = v.toRuntimeError(r)
	}
}

// (Table indexing now goes through indexMM / newIndexMM in metatable.go.)

// SetGlobal is a convenience helper for embedding code: register `name` in
// the globals table without going through bytecode.
func (v *VM) SetGlobal(name string, val Value) {
	v.Globals.Set(name, val)
}

// CallFrames returns the live call-frame stack, outermost first. The slice
// shares backing storage with the VM — callers must not mutate or retain it
// past the current callback. Exposed for the `debug` native module so it can
// implement traceback / getinfo without living inside this package.
func (v *VM) CallFrames() []*CallFrame {
	return v.frames
}

// CallValue invokes `fn` with `args` from Go code. Useful for embedding and
// for stdlib helpers that need to call back into Lua (e.g. ipairs internals).
func (v *VM) CallValue(fn Value, args []Value, nresults int) []Value {
	switch g := fn.(type) {
	case *Closure:
		base := len(v.Stack)
		v.callClosure(g, args, nresults)
		out := append([]Value(nil), v.Stack[base:]...)
		v.Stack = v.Stack[:base]
		return out
	case *GoFunc:
		raw := g.Fn(v, args)
		if nresults < 0 {
			return raw
		}
		out := make([]Value, nresults)
		for i := range nresults {
			if i < len(raw) {
				out[i] = raw[i]
			}
		}
		return out
	}
	panic(Errorf("attempt to call a %s value", TypeName(fn)))
}
