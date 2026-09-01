package vm

import (
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

type VM struct {
	Stack   []Value
	Globals *Table

	stringMeta *Table

	mainThread *Thread
	currentCo  *Coroutine

	frames   []*CallFrame
	openUpvs []*Upvalue

	callMarks []int

	framePool []*CallFrame

	retScratch []Value

	mode parser.Mode
}

func New() *VM {
	v := &VM{
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

func (v *VM) Run(main *bytecode.InstructionSet) (err error) {
	defer v.recoverToError(&err)

	cl := &Closure{Proto: main}
	v.callClosure(cl, nil, 0)
	return nil
}

func (v *VM) RunMainChunkWithResults(main *bytecode.InstructionSet) (results []Value, err error) {
	defer v.recoverToError(&err)

	base := len(v.Stack)
	cl := &Closure{Proto: main}
	v.callClosure(cl, nil, -1)
	results = append([]Value(nil), v.Stack[base:]...)
	v.Stack = v.Stack[:base]
	return results, nil
}

func (v *VM) recoverToError(err *error) {
	if r := recover(); r != nil {
		*err = v.toRuntimeError(r)
	}
}

func (v *VM) SetGlobal(name string, val Value) {
	v.Globals.Set(name, val)
}

func (v *VM) CallFrames() []*CallFrame {
	return v.frames
}

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
