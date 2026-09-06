package vm

import "github.com/hilthontt/luascript/internal/compiler/bytecode"

func (v *VM) callClosure(cl *Closure, args []Value, nresults int) {
	if len(v.frames) >= maxCallDepth {
		panic(LuaError("stack overflow"))
	}
	base := len(v.Stack)
	nparams := cl.Proto.NumParams
	for i := range nparams {
		if i < len(args) {
			v.push(args[i])
		} else {
			v.push(nil)
		}
	}
	var varargs []Value
	if cl.Proto.IsVararg && len(args) > nparams {
		varargs = append(varargs, args[nparams:]...)
	}
	if !cl.Proto.LocalsResolved() {
		n, hasTry := scanProto(cl.Proto)
		if n > cl.Proto.NumLocals {
			cl.Proto.NumLocals = n
		}
		cl.Proto.SetHasTry(hasTry)
		cl.Proto.MarkLocalsResolved()
	}
	numLocals := cl.Proto.NumLocals
	for len(v.Stack)-base < numLocals {
		v.push(nil)
	}

	frame := v.acquireFrame(cl, base, len(v.Stack), nresults, varargs)
	v.frames = append(v.frames, frame)
	v.exec(len(v.frames))
}

func scanProto(p *bytecode.InstructionSet) (numLocals int, hasTry bool) {
	maxSlot := -1
	bump := func(n int) {
		if n > maxSlot {
			maxSlot = n
		}
	}
	for _, ins := range p.Instructions {
		switch ins.Opcode {
		case bytecode.SetLocal, bytecode.GetLocal:
			bump(int(ins.A))
		case bytecode.ForPrep, bytecode.ForLoop:
			bump(int(ins.A) + 3)
		case bytecode.TForCall:
			bump(int(ins.A) + 2 + int(ins.B))
		case bytecode.TForLoop:
			bump(int(ins.A) + 3)
		case bytecode.Try:
			hasTry = true
		}
	}
	return maxSlot + 1, hasTry
}

func (v *VM) makeClosure(parent *CallFrame, proto *bytecode.InstructionSet) *Closure {
	if len(proto.Upvalues) == 0 {
		return &Closure{Proto: proto}
	}
	cl := &Closure{Proto: proto, Upvalues: make([]*Upvalue, len(proto.Upvalues))}
	for i, desc := range proto.Upvalues {
		if desc.InStack {
			cl.Upvalues[i] = v.findOrCreateOpenUpvalue(parent.Base + desc.Index)
		} else {
			cl.Upvalues[i] = parent.Closure.Upvalues[desc.Index]
		}
	}
	return cl
}

func (v *VM) doCall(nargs, nresults int) {
	var argsStart int
	if nargs < 0 {
		if len(v.callMarks) == 0 {
			panic("vm: Call with nargs=-1 but no MarkArgs mark on stack")
		}
		argsStart = v.callMarks[len(v.callMarks)-1]
		v.callMarks = v.callMarks[:len(v.callMarks)-1]
		nargs = len(v.Stack) - argsStart
	} else {
		argsStart = len(v.Stack) - nargs
	}
	fn := v.Stack[argsStart-1]

	switch g := fn.(type) {
	case *Closure:
		args := v.Stack[argsStart : argsStart+nargs]
		v.Stack = v.Stack[:argsStart-1]
		v.callClosureWithResults(g, args, nresults)
	case *GoFunc:
		args := append([]Value(nil), v.Stack[argsStart:argsStart+nargs]...)
		v.Stack = v.Stack[:argsStart-1]
		results := g.Fn(v, args)
		v.pushResults(results, nresults)
	default:
		args := append([]Value(nil), v.Stack[argsStart:argsStart+nargs]...)
		v.Stack = v.Stack[:argsStart-1]
		if results, ok := v.callMMIfNotFunction(fn, args, nresults); ok {
			v.pushResults(results, nresults)
			return
		}
		panic(Errorf("attempt to call a %s value", TypeName(fn)))
	}
}

func (v *VM) callClosureWithResults(cl *Closure, args []Value, nresults int) {
	resultsBase := len(v.Stack)
	v.callClosure(cl, args, nresults)
	_ = resultsBase
}

func (v *VM) pushResults(results []Value, nresults int) {
	if nresults < 0 {
		for _, r := range results {
			v.push(r)
		}
		return
	}
	for i := 0; i < nresults; i++ {
		if i < len(results) {
			v.push(results[i])
		} else {
			v.push(nil)
		}
	}
}

func (v *VM) doReturn(f *CallFrame, count int) {
	var start int
	if count < 0 {
		start = f.Base + f.Closure.Proto.NumLocals
	} else {
		start = len(v.Stack) - count
	}
	v.retScratch = append(v.retScratch[:0], v.Stack[start:]...)
	v.unwindFrame(f, v.retScratch)
}

func (v *VM) unwindFrame(f *CallFrame, rets []Value) {
	f.handlers = nil
	if len(f.tbc) > 0 || len(f.Deferred) > 0 {
		rets = append([]Value(nil), rets...)
		v.closeTBC(f, len(f.tbc), nil)
		v.runDeferred(f)
	}
	v.closeUpvaluesAbove(f.Base)
	v.Stack = v.Stack[:f.Base]
	v.frames = v.frames[:len(v.frames)-1]
	v.pushResults(rets, f.NResults)
	v.releaseFrame(f)
}

func (v *VM) runDeferred(f *CallFrame) {
	deferred := f.Deferred
	f.Deferred = nil
	for i := len(deferred) - 1; i >= 0; i-- {
		v.callClosure(deferred[i], nil, 0)
	}
}

func (v *VM) runDeferredSafely(f *CallFrame) {
	deferred := f.Deferred
	f.Deferred = nil
	for i := len(deferred) - 1; i >= 0; i-- {
		fd, st := len(v.frames), len(v.Stack)
		func() {
			defer func() {
				if recover() != nil {
					v.closeUpvaluesAbove(st)
					v.frames = v.frames[:fd]
					v.Stack = v.Stack[:st]
				}
			}()
			v.callClosure(deferred[i], nil, 0)
		}()
	}
}
