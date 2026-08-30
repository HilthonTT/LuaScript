package vm

// Calls and returns: pushing frames, building closures, unwinding, and deferred calls.

import "github.com/hilthontt/luascript/internal/compiler/bytecode"

// callClosure pushes a new frame for `cl` with `args` and runs until that
// frame returns. Results are placed on the stack starting at the saved top
// before the call. nresults is the caller's expected count (-1 = all).
func (v *VM) callClosure(cl *Closure, args []Value, nresults int) {
	if len(v.frames) >= maxCallDepth {
		panic(LuaError("stack overflow"))
	}
	base := len(v.Stack)
	// Place declared parameters; surplus args are folded into varargs.
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
	// Reserve slot space for the function's declared locals beyond params.
	// The main chunk's NumLocals is left at 0 by the bytecode generator
	// (popFunction is the only writer and it never runs for the chunk), so
	// fall back to a one-time scan of the instruction stream. The result is
	// cached on the proto via localsResolved — without that flag this scan
	// would run on every single call (a full instruction-stream walk).
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
	// Run until this frame (and everything it calls into) returns. The
	// caller's frame is at frames[len-2], so we stop as soon as the depth
	// drops below this newly-pushed frame's index + 1.
	v.exec(len(v.frames))
}

// scanProto walks p's instruction stream once, deriving the two facts the VM
// cannot read off the proto directly. Both are cached on the proto behind
// LocalsResolved, so this runs at most once per prototype.
//
// numLocals is the largest local slot referenced (either via Set/GetLocal or
// implicitly by the for-loop opcodes, which use baseSlot..baseSlot+3 for
// numeric for — three hidden control slots plus the visible variable — and
// baseSlot..baseSlot+2+nresults for the generic for), plus one
// — i.e. the count of slots to reserve at frame entry. It is needed because the
// generator leaves the main chunk's NumLocals at 0.
//
// hasTry reports whether the body installs any error handler, which decides
// whether calls into it need the recover-installing exec path.
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

// Closure construction

// makeClosure builds a closure for `proto` capturing upvalues per its
// UpvalueDesc table. InStack=true descriptors look up the immediately
// enclosing function's stack slot; InStack=false descriptors share an
// upvalue from the enclosing closure.
func (v *VM) makeClosure(parent *CallFrame, proto *bytecode.InstructionSet) *Closure {
	// Skip the Upvalues slice allocation entirely for closures that capture
	// nothing — common for top-level local functions and module-scope
	// helpers. nil slice is fine: every reader uses range/len which handle it.
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

// Calls and returns

// doCall pops the function + nargs args and invokes it. The function lives
// at Stack[sp-nargs-1]; arguments at Stack[sp-nargs..sp]. Results land on
// the stack adjusted to nresults.
func (v *VM) doCall(nargs, nresults int) {
	var argsStart int
	if nargs < 0 {
		// Variadic call: codegen emitted MarkArgs before the args so the
		// args base could be recovered now. Pop the latest mark; argsStart
		// is the slot of the first arg (== fnSlot+1).
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
		// callClosure consumes its `args` slice immediately by copying each
		// value into the new frame's local slots (and into a fresh varargs
		// slice if applicable), so we can hand it a stack-aliased view and
		// skip the per-call slice allocation. The aliasing is safe because
		// the param-push writes to slots strictly *before* the slots being
		// read (target = source - 1 in stack-index terms), and varargs are
		// fully copied before local-slot padding overwrites the source.
		args := v.Stack[argsStart : argsStart+nargs]
		v.Stack = v.Stack[:argsStart-1]
		v.callClosureWithResults(g, args, nresults)
	case *GoFunc:
		// GoFunc bodies are foreign code — they may retain the slice (e.g.
		// to return it as their result, or stash it). Pay the copy at the
		// FFI boundary so subsequent stack activity can't tear the values.
		args := append([]Value(nil), v.Stack[argsStart:argsStart+nargs]...)
		v.Stack = v.Stack[:argsStart-1]
		results := g.Fn(v, args)
		v.pushResults(results, nresults)
	default:
		// __call metamethod fallback: any non-function with a __call entry
		// in its metatable can be invoked, with the original target spliced
		// in as the first argument. callMMIfNotFunction reuses the args
		// list across an unknown number of intermediate calls, so copy.
		args := append([]Value(nil), v.Stack[argsStart:argsStart+nargs]...)
		v.Stack = v.Stack[:argsStart-1]
		if results, ok := v.callMMIfNotFunction(fn, args, nresults); ok {
			v.pushResults(results, nresults)
			return
		}
		panic(Errorf("attempt to call a %s value", TypeName(fn)))
	}
}

// callClosureWithResults invokes a script function and adjusts its return
// values to nresults on completion.
func (v *VM) callClosureWithResults(cl *Closure, args []Value, nresults int) {
	resultsBase := len(v.Stack)
	v.callClosure(cl, args, nresults)
	// callClosure → exec → doReturn already appended results and adjusted
	// to nresults (when nresults != -1). Nothing more to do.
	_ = resultsBase
}

// pushResults is used for GoFunc returns: place `results` on the stack and
// adjust to nresults (-1 means "all").
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

// doReturn unwinds the active frame, placing its returned values where the
// caller expects them.
func (v *VM) doReturn(f *CallFrame, count int) {
	// Locate the return values on the stack.
	var start int
	if count < 0 {
		// -1: every value pushed past the locals area is a return value.
		start = f.Base + f.Closure.Proto.NumLocals
	} else {
		// Top `count` stack entries are the return values.
		start = len(v.Stack) - count
	}
	// Copy them into the reused scratch buffer before unwindFrame truncates
	// the stack and pushResults re-appends over the vacated region. The
	// scratch buffer has its own backing array, so the copy survives the
	// truncate/re-append without a fresh allocation per return.
	v.retScratch = append(v.retScratch[:0], v.Stack[start:]...)
	v.unwindFrame(f, v.retScratch)
}

// unwindFrame closes any open upvalues at-or-above this frame's base, pops
// the frame, truncates the stack back to f.Base, and pushes the returned
// values adjusted to the caller's expected nresults.
//
// When the popped frame is the last one (Run on the main chunk, or
// CallValue with the coroutine goroutine's only frame) we still go through
// pushResults — Run sets NResults=0 so nothing is left behind, while
// CallValue sets NResults=-1 so all returned values land on the stack
// where the caller can read them via Stack[base:].
func (v *VM) unwindFrame(f *CallFrame, rets []Value) {
	// The frame is exiting: any `try` regions still open in it are gone. Drop
	// them before running defers, or a defer that errors during a `return`
	// out of a try would dispatch back into this frame's own catch clause —
	// resurrecting a function that already committed its return value.
	f.handlers = nil
	if len(f.Deferred) > 0 {
		// Deferred calls return through doReturn, which reuses v.retScratch —
		// the same buffer `rets` usually aliases. Copy the return values into a
		// private slice so a deferred call can't clobber them, then run the
		// defers while f's locals and open upvalues are still live on the
		// stack (so the deferred call observes the function's final state).
		rets = append([]Value(nil), rets...)
		v.runDeferred(f)
	}
	v.closeUpvaluesAbove(f.Base)
	v.Stack = v.Stack[:f.Base]
	v.frames = v.frames[:len(v.frames)-1]
	v.pushResults(rets, f.NResults)
	v.releaseFrame(f)
}

// runDeferred runs f's deferred closures in last-in-first-out order, matching
// Go. It clears f.Deferred up front so a panic from one deferred call cannot
// cause the same defers to run twice (once here, once again from safeCall's
// error-unwind path). A panic from a deferred call propagates — an
// uncaught cleanup failure should surface, and any enclosing pcall will run
// the remaining frames' defers via runDeferredSafely.
func (v *VM) runDeferred(f *CallFrame) {
	deferred := f.Deferred
	f.Deferred = nil
	for i := len(deferred) - 1; i >= 0; i-- {
		v.callClosure(deferred[i], nil, 0)
	}
}

// runDeferredSafely is the error-unwind counterpart of runDeferred: it runs
// f's defers but contains a panic from any single one, restoring the
// frame/stack/upvalue state to the pre-call snapshot so the next defer — and
// the surrounding pcall unwind — still proceed cleanly. Used only while
// already recovering from an error, so a faulty cleanup can't replace the
// original error or leave the VM half-unwound.
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
