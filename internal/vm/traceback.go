package vm

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/bytecode"
)

type TracebackEntry struct {
	Source   string
	Line     int
	Function string
	IsMain   bool
	Elided   int
}

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

const (
	tracebackHead = 10
	tracebackTail = 11
)

func (v *VM) Traceback(skip int) []TracebackEntry {
	if skip < 0 {
		skip = 0
	}
	return v.tracebackRange(0, len(v.frames)-skip)
}

func (v *VM) tracebackFrom(base int) []TracebackEntry {
	if base < 0 {
		base = 0
	}
	return v.tracebackRange(base, len(v.frames))
}

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

type RuntimeError struct {
	Value Value
	Msg   string
	Stack []TracebackEntry
}

func (e *RuntimeError) Error() string {
	if len(e.Stack) <= 1 {
		return e.Msg
	}
	tb := FormatTraceback(e.Stack)
	if tb == "" {
		return e.Msg
	}
	return e.Msg + "\n" + tb
}

func (e *RuntimeError) Message() string { return e.Msg }

func (e *RuntimeError) Traceback() string { return FormatTraceback(e.Stack) }

func (v *VM) toRuntimeError(r any) *RuntimeError {
	val := v.errorValue(r)
	return &RuntimeError{
		Value: val,
		Msg:   ToStringMM(v, val),
		Stack: v.Traceback(0),
	}
}
