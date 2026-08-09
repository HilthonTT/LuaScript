package vm

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/bytecode"
)

// This file is the VM's error-reporting surface: turning a panic that is
// unwinding the Go stack into a Lua error value carrying a source position and
// a snapshot of the Lua call stack it came from.
//
// The snapshot has to be taken before anything truncates v.frames. There are
// exactly four places where an in-flight error stops unwinding and the frames
// it came from are discarded:
//
//   - safeCall            — pcall / xpcall / VM.SafeCall
//   - dispatchToHandler   — a `try` region catching it
//   - goroutineBody       — a coroutine dying, reported through resume
//   - recoverToError      — the error reaching the host uncaught
//
// Each of those calls errorValue (or, at the top level, toRuntimeError) as the
// first thing in its recover, before any unwinding. Everywhere else the panic
// is re-panicked untouched, so the deepest boundary is always the one that
// records the position — which is the raise site.

// TracebackEntry is one Lua activation in a captured call stack, or — when
// Elided is non-zero — the marker standing in for a run of frames the capture
// skipped over.
type TracebackEntry struct {
	Source   string // chunk name, e.g. "examples/01_basics.lsc"
	Line     int    // source line the frame was executing, 0 if unknown
	Function string // proto name; "" for a main chunk
	IsMain   bool   // this frame is a chunk body rather than a function
	Elided   int    // >0: not a frame, but "N levels omitted here"
}

// String renders one traceback line in Lua's shape, without the leading tab.
func (e TracebackEntry) String() string {
	if e.Elided > 0 {
		return fmt.Sprintf("... (skipping %d levels)", e.Elided)
	}
	var b strings.Builder
	b.WriteString(e.Source)
	if e.Line > 0 {
		fmt.Fprintf(&b, ":%d", e.Line)
	}
	b.WriteString(": in ")
	switch {
	case e.IsMain:
		b.WriteString("main chunk")
	case e.Function == "":
		b.WriteString("function <anonymous>")
	default:
		b.WriteString("function '")
		b.WriteString(e.Function)
		b.WriteByte('\'')
	}
	return b.String()
}

// Frame budget for a captured traceback. maxCallDepth is in the tens of
// thousands, so a runaway recursion must not render (or even allocate) one
// line per frame. Following Lua, we keep the innermost frames — where the
// fault is — and the outermost ones — which say how execution got there —
// and collapse the repetitive middle into a single marker.
const (
	tracebackHead = 10 // innermost frames kept
	tracebackTail = 11 // outermost frames kept
)

// Traceback captures the Lua call stack, innermost frame first, skipping the
// `skip` innermost frames. GoFuncs do not push frames, so a builtin that
// raises is represented by its Lua caller — the same convention Lua's own
// error levels use.
//
// A stack deeper than tracebackHead+tracebackTail is abbreviated with an
// Elided marker in the middle.
//
// The result is a copy: safe to keep after the VM unwinds, which is the whole
// point, since it is captured mid-panic and read afterwards.
func (v *VM) Traceback(skip int) []TracebackEntry {
	if skip < 0 {
		skip = 0
	}
	return v.tracebackRange(0, len(v.frames)-skip)
}

// tracebackFrom captures only the activations at or above frame index base —
// the ones a protected call pushed itself, with its caller's stack left out.
// SafeCallTrace uses it so a host running a callback (a test body, an event
// handler) reports the callback's own stack and not the chunk that installed
// it.
func (v *VM) tracebackFrom(base int) []TracebackEntry {
	if base < 0 {
		base = 0
	}
	return v.tracebackRange(base, len(v.frames))
}

// tracebackRange renders frames [lo, hi) innermost-first, abbreviating the
// middle when the range exceeds the head+tail budget.
func (v *VM) tracebackRange(lo, hi int) []TracebackEntry {
	if lo < 0 {
		lo = 0
	}
	if hi > len(v.frames) {
		hi = len(v.frames)
	}
	n := hi - lo
	if n <= 0 {
		return nil
	}
	// frames is outermost-first; walking down from index hi-1 yields
	// innermost-first, which is the order a traceback reads in.
	if n <= tracebackHead+tracebackTail {
		entries := make([]TracebackEntry, 0, n)
		for i := hi - 1; i >= lo; i-- {
			entries = append(entries, frameEntry(v.frames[i]))
		}
		return entries
	}
	entries := make([]TracebackEntry, 0, tracebackHead+tracebackTail+1)
	for i := hi - 1; i >= hi-tracebackHead; i-- {
		entries = append(entries, frameEntry(v.frames[i]))
	}
	entries = append(entries, TracebackEntry{Elided: n - tracebackHead - tracebackTail})
	for i := lo + tracebackTail - 1; i >= lo; i-- {
		entries = append(entries, frameEntry(v.frames[i]))
	}
	return entries
}

// frameEntry describes a single live frame.
func frameEntry(f *CallFrame) TracebackEntry {
	if f == nil || f.Closure == nil || f.Closure.Proto == nil {
		return TracebackEntry{Source: "?"}
	}
	p := f.Closure.Proto
	e := TracebackEntry{
		Source: p.Source(),
		Line:   frameLine(f),
		IsMain: p.Type() == bytecode.Program,
	}
	if !e.IsMain {
		e.Function = p.Name()
	}
	return e
}

// frameLine resolves the source line a frame is parked on. The dispatch loop
// increments IP before executing an instruction, so the one that is running —
// and therefore the one that raised — sits at IP-1.
func frameLine(f *CallFrame) int {
	if f == nil || f.Closure == nil || f.Closure.Proto == nil {
		return 0
	}
	ins := f.Closure.Proto.Instructions
	idx := f.IP - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ins) {
		idx = len(ins) - 1
	}
	if idx < 0 {
		return 0
	}
	return ins[idx].SourceLine()
}

// FormatTraceback renders captured entries under Lua's "stack traceback:"
// header. Returns "" for an empty capture so callers can append it
// unconditionally without emitting a bare header.
func FormatTraceback(entries []TracebackEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("stack traceback:")
	for _, e := range entries {
		b.WriteString("\n\t")
		b.WriteString(e.String())
	}
	return b.String()
}

// where renders the "<source>:<line>: " position prefix Lua puts on runtime
// error messages, resolved `level` frames up: level 1 is the innermost Lua
// frame, level 2 its caller, and so on. Returns "" when the level has no frame
// or the frame has no line information.
func (v *VM) where(level int) string {
	idx := len(v.frames) - level
	if idx < 0 || idx >= len(v.frames) {
		return ""
	}
	f := v.frames[idx]
	if f.Closure == nil || f.Closure.Proto == nil {
		return ""
	}
	line := frameLine(f)
	if line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d: ", f.Closure.Proto.Source(), line)
}

// errorValue converts a recovered panic into the Lua error value a protected
// call should surface, stamping the raise position onto it when appropriate.
//
// The distinction that drives this is the panic's type. LuaError (and a bare
// Go error) means the VM itself raised — "attempt to index a nil value" — and
// Lua prefixes those with the position, which nothing has done yet. luaError
// means a script raised via error()/assert()/throw, where positioning is the
// raiser's business: error() already applied it at the requested level, and
// throw is deliberately verbatim.
//
// Callers must invoke this before unwinding: it reads v.frames.
func (v *VM) errorValue(r any) Value {
	switch e := r.(type) {
	case luaError:
		return e.value
	case LuaError:
		return v.where(1) + string(e)
	case error:
		return v.where(1) + e.Error()
	default:
		return v.where(1) + fmt.Sprintf("%v", r)
	}
}

// RuntimeError is an uncaught Lua error as it reaches the host: the original
// error value, the positioned message, and the Lua call stack at the raise
// point. Error() renders message and traceback together, which is what a CLI
// wants to print; Message() is the bare positioned message.
type RuntimeError struct {
	Value Value
	Msg   string
	Stack []TracebackEntry
}

func (e *RuntimeError) Error() string {
	// A stack that is nothing but the main chunk tells the reader exactly
	// what the message's own position prefix already did, so don't repeat it.
	// This is the common shape for REPL input and one-line scripts.
	if len(e.Stack) <= 1 {
		return e.Msg
	}
	tb := FormatTraceback(e.Stack)
	if tb == "" {
		return e.Msg
	}
	return e.Msg + "\n" + tb
}

// Message returns the error message without the traceback.
func (e *RuntimeError) Message() string { return e.Msg }

// Traceback returns the rendered traceback without the message, or "" when
// no frames were captured.
func (e *RuntimeError) Traceback() string { return FormatTraceback(e.Stack) }

// toRuntimeError builds the host-facing error for a panic that escaped every
// protected call. Must run before v.frames is unwound.
func (v *VM) toRuntimeError(r any) *RuntimeError {
	val := v.errorValue(r)
	return &RuntimeError{
		Value: val,
		// ToStringMM honours __tostring, so error(someObject) reaching the
		// top level prints the way the script intended rather than as a
		// table address.
		Msg:   ToStringMM(v, val),
		Stack: v.Traceback(0),
	}
}
