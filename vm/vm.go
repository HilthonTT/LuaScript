package vm

import (
	"fmt"

	"github.com/hilthontt/sakura-lang/compiler/bytecode"
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
}

// New creates a fresh VM with an empty globals table.
func New() *VM {
	v := &VM{
		Stack:      make([]Value, 0, 256),
		Globals:    NewTable(0, 32),
		mainThread: &Thread{},
	}
	registerStdlib(v)
	registerCoroutineLibrary(v)
	registerLibraryModules(v)
	return v
}

// Run loads `main` as the top-level chunk and executes it. The chunk runs
// with no arguments and discards results. Any runtime error surfaces as a
// non-nil error.
func (v *VM) Run(main *bytecode.InstructionSet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case luaError:
				err = e
			case error:
				err = e
			default:
				err = fmt.Errorf("vm panic: %v", r)
			}
		}
	}()

	cl := &Closure{Proto: main}
	v.callClosure(cl, nil, 0)
	return nil
}

// callClosure pushes a new frame for `cl` with `args` and runs until that
// frame returns. Results are placed on the stack starting at the saved top
// before the call. nresults is the caller's expected count (-1 = all).
func (v *VM) callClosure(cl *Closure, args []Value, nresults int) {
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
	// fall back to a one-time scan of the instruction stream when needed.
	numLocals := cl.Proto.NumLocals
	if numLocals < computeMaxLocalSlots(cl.Proto) {
		numLocals = computeMaxLocalSlots(cl.Proto)
		cl.Proto.NumLocals = numLocals
	}
	for len(v.Stack)-base < numLocals {
		v.push(nil)
	}

	frame := &CallFrame{
		Closure:  cl,
		IP:       0,
		Base:     base,
		Top:      len(v.Stack),
		NResults: nresults,
		Varargs:  varargs,
	}
	v.frames = append(v.frames, frame)
	// Run until this frame (and everything it calls into) returns. The
	// caller's frame is at frames[len-2], so we stop as soon as the depth
	// drops below this newly-pushed frame's index + 1.
	v.exec(len(v.frames))
}

// computeMaxLocalSlots scans p's instruction stream to find the largest
// local slot referenced (either via Set/GetLocal or implicitly via the
// for-loop opcodes which use baseSlot..baseSlot+2 for numeric for and
// baseSlot..baseSlot+2+nresults for the generic for). Returns slot+1 i.e.
// the count of slots the function needs reserved at frame entry.
func computeMaxLocalSlots(p *bytecode.InstructionSet) int {
	maxSlot := -1
	bump := func(n int) {
		if n > maxSlot {
			maxSlot = n
		}
	}
	for _, ins := range p.Instructions {
		switch ins.Opcode {
		case bytecode.SetLocal, bytecode.GetLocal:
			if s, ok := ins.Params[0].(int); ok {
				bump(s)
			}
		case bytecode.ForPrep, bytecode.ForLoop:
			if s, ok := ins.Params[0].(int); ok {
				bump(s + 2)
			}
		case bytecode.TForCall:
			if s, ok := ins.Params[0].(int); ok {
				if n, ok := ins.Params[1].(int); ok {
					bump(s + 2 + n)
				} else {
					bump(s + 2)
				}
			}
		case bytecode.TForLoop:
			if s, ok := ins.Params[0].(int); ok {
				bump(s + 3)
			}
		}
	}
	return maxSlot + 1
}

func (v *VM) push(x Value) { v.Stack = append(v.Stack, x) }
func (v *VM) pop() Value {
	n := len(v.Stack) - 1
	x := v.Stack[n]
	v.Stack = v.Stack[:n]
	return x
}
func (v *VM) popN(n int) {
	if n <= 0 {
		return
	}
	v.Stack = v.Stack[:len(v.Stack)-n]
}

// local returns a pointer to slot `i` in the active frame's locals.
func (v *VM) localAt(f *CallFrame, i int) *Value {
	return &v.Stack[f.Base+i]
}

// findOrCreateOpenUpvalue returns the open upvalue tracking stack[index],
// creating one if none exists. Open upvalues are kept in v.openUpvs sorted
// by ascending Index so close-on-return can scan from the tail.
func (v *VM) findOrCreateOpenUpvalue(index int) *Upvalue {
	for _, u := range v.openUpvs {
		if u.Stack == &v.Stack && u.Index == index {
			return u
		}
	}
	u := &Upvalue{Stack: &v.Stack, Index: index}
	v.openUpvs = append(v.openUpvs, u)
	return u
}

// closeUpvaluesAbove closes (snapshots into Closed) every open upvalue whose
// stack slot is at or above `index`. Called when a frame returns.
func (v *VM) closeUpvaluesAbove(index int) {
	keep := v.openUpvs[:0]
	for _, u := range v.openUpvs {
		if u.Index >= index {
			u.Closed = (*u.Stack)[u.Index]
			u.Stack = nil
			continue
		}
		keep = append(keep, u)
	}
	v.openUpvs = keep
}

// exec drives the interpreter until the frame stack drops below `entryDepth`.
// `entryDepth` is the number of frames present immediately after the call
// site pushed its target frame; when that frame returns, len(v.frames) drops
// below entryDepth and exec exits, returning control to the Go caller.
func (v *VM) exec(entryDepth int) {
	for len(v.frames) >= entryDepth {
		f := v.frames[len(v.frames)-1]
		if f.IP >= len(f.Closure.Proto.Instructions) {
			// Defensive — every well-formed proto ends with Leave/Return.
			v.unwindFrame(f)
			continue
		}
		ins := f.Closure.Proto.Instructions[f.IP]
		f.IP++
		v.dispatch(f, ins)
	}
}

// dispatch executes a single instruction.
func (v *VM) dispatch(f *CallFrame, ins *bytecode.Instruction) {
	switch ins.Opcode {

	// ----- constants & literals -----
	case bytecode.LoadNil:
		count := ins.Params[0].(int)
		for i := 0; i < count; i++ {
			v.push(nil)
		}
	case bytecode.LoadTrue:
		v.push(true)
	case bytecode.LoadFalse:
		v.push(false)
	case bytecode.LoadInt:
		v.push(ins.Params[0].(int64))
	case bytecode.LoadFloat:
		v.push(ins.Params[0].(float64))
	case bytecode.LoadString:
		v.push(ins.Params[0].(string))
	case bytecode.LoadVararg:
		count := ins.Params[0].(int)
		va := f.Varargs
		switch {
		case count < 0:
			for _, x := range va {
				v.push(x)
			}
		default:
			for i := 0; i < count; i++ {
				if i < len(va) {
					v.push(va[i])
				} else {
					v.push(nil)
				}
			}
		}
	case bytecode.Closure:
		idx := ins.Params[0].(int)
		proto := f.Closure.Proto.Protos[idx]
		v.push(v.makeClosure(f, proto))

	// ----- variables -----
	case bytecode.GetLocal:
		slot := ins.Params[0].(int)
		v.push(*v.localAt(f, slot))
	case bytecode.SetLocal:
		slot := ins.Params[0].(int)
		*v.localAt(f, slot) = v.pop()
	case bytecode.GetUpvalue:
		idx := ins.Params[0].(int)
		v.push(f.Closure.Upvalues[idx].Get())
	case bytecode.SetUpvalue:
		idx := ins.Params[0].(int)
		f.Closure.Upvalues[idx].Set(v.pop())
	case bytecode.GetGlobal:
		name := ins.Params[0].(string)
		v.push(v.Globals.Get(name))
	case bytecode.SetGlobal:
		name := ins.Params[0].(string)
		v.Globals.Set(name, v.pop())

	// ----- tables -----
	case bytecode.NewTable:
		ah := ins.Params[0].(int)
		hh := ins.Params[1].(int)
		v.push(NewTable(ah, hh))
	case bytecode.GetTable:
		key := v.pop()
		obj := v.pop()
		v.push(v.indexMM(obj, key))
	case bytecode.SetTable:
		val := v.pop()
		key := v.pop()
		obj := v.pop()
		v.newIndexMM(obj, key, val)
	case bytecode.GetField:
		key := ins.Params[0].(string)
		obj := v.pop()
		v.push(v.indexMM(obj, key))
	case bytecode.SetField:
		key := ins.Params[0].(string)
		val := v.pop()
		obj := v.pop()
		v.newIndexMM(obj, key, val)
	case bytecode.Self:
		key := ins.Params[0].(string)
		obj := v.Stack[len(v.Stack)-1] // peek; we want both method and obj
		v.Stack[len(v.Stack)-1] = v.indexMM(obj, key)
		v.push(obj)
	case bytecode.SetList:
		count := ins.Params[0].(int)
		offset := ins.Params[1].(int)
		// Stack layout: [..., table, v1, v2, ..., vCount]
		valuesStart := len(v.Stack) - count
		t := v.Stack[valuesStart-1].(*Table)
		for i := 0; i < count; i++ {
			t.Set(int64(offset+i+1), v.Stack[valuesStart+i])
		}
		v.popN(count)

	// ----- arithmetic ----- (each routes through metatable.go for metamethod fallback)
	case bytecode.Add:
		b := v.pop()
		a := v.pop()
		v.push(v.arithMM(a, b, "+", metaAdd))
	case bytecode.Sub:
		b := v.pop()
		a := v.pop()
		v.push(v.arithMM(a, b, "-", metaSub))
	case bytecode.Mul:
		b := v.pop()
		a := v.pop()
		v.push(v.arithMM(a, b, "*", metaMul))
	case bytecode.Div:
		b := v.pop()
		a := v.pop()
		v.push(v.arithDivMM(a, b))
	case bytecode.FloorDiv:
		b := v.pop()
		a := v.pop()
		v.push(v.arithFloorDivMM(a, b))
	case bytecode.Mod:
		b := v.pop()
		a := v.pop()
		v.push(v.arithMM(a, b, "%", metaMod))
	case bytecode.Pow:
		b := v.pop()
		a := v.pop()
		v.push(v.arithPowMM(a, b))
	case bytecode.Neg:
		v.push(v.arithNegMM(v.pop()))

	// ----- bitwise -----
	case bytecode.BitAnd:
		b := v.pop()
		a := v.pop()
		v.push(v.bitwiseMM(a, b, bitAnd, metaBAnd))
	case bytecode.BitOr:
		b := v.pop()
		a := v.pop()
		v.push(v.bitwiseMM(a, b, bitOr, metaBOr))
	case bytecode.BitXor:
		b := v.pop()
		a := v.pop()
		v.push(v.bitwiseMM(a, b, bitXor, metaBXor))
	case bytecode.Shl:
		b := v.pop()
		a := v.pop()
		v.push(v.bitwiseMM(a, b, shl, metaShl))
	case bytecode.Shr:
		b := v.pop()
		a := v.pop()
		v.push(v.bitwiseMM(a, b, shr, metaShr))
	case bytecode.BitNot:
		v.push(v.bitNotMM(v.pop()))

	// ----- string / length -----
	case bytecode.Concat:
		count := ins.Params[0].(int)
		// Right-associative: reduce pairwise from the right so __concat
		// metamethods see the same operand pairing as a chained `..`.
		start := len(v.Stack) - count
		acc := v.Stack[len(v.Stack)-1]
		for i := count - 2; i >= 0; i-- {
			acc = v.concatMM(v.Stack[start+i], acc)
		}
		v.popN(count)
		v.push(acc)
	case bytecode.Len:
		v.push(v.lenMM(v.pop()))

	// ----- comparison -----
	case bytecode.Eq:
		b := v.pop()
		a := v.pop()
		v.push(v.equalMM(a, b))
	case bytecode.NotEq:
		b := v.pop()
		a := v.pop()
		v.push(!v.equalMM(a, b))
	case bytecode.Lt:
		b := v.pop()
		a := v.pop()
		v.push(v.lessMM(a, b))
	case bytecode.Le:
		b := v.pop()
		a := v.pop()
		v.push(v.lessOrEqualMM(a, b))
	case bytecode.Gt:
		b := v.pop()
		a := v.pop()
		v.push(v.lessMM(b, a))
	case bytecode.Ge:
		b := v.pop()
		a := v.pop()
		v.push(v.lessOrEqualMM(b, a))

	// ----- logical -----
	case bytecode.Not:
		v.push(!IsTruthy(v.pop()))

	// ----- control flow -----
	case bytecode.Jump:
		f.IP = ins.Params[0].(int)
	case bytecode.JumpIfFalse:
		x := v.pop()
		if !IsTruthy(x) {
			f.IP = ins.Params[0].(int)
		}
	case bytecode.JumpIfTrue:
		x := v.pop()
		if IsTruthy(x) {
			f.IP = ins.Params[0].(int)
		}
	case bytecode.JumpIfFalseKeep:
		x := v.Stack[len(v.Stack)-1]
		if !IsTruthy(x) {
			f.IP = ins.Params[0].(int)
		} else {
			v.pop()
		}
	case bytecode.JumpIfTrueKeep:
		x := v.Stack[len(v.Stack)-1]
		if IsTruthy(x) {
			f.IP = ins.Params[0].(int)
		} else {
			v.pop()
		}

	// ----- calls / returns -----
	case bytecode.Call:
		nargs := ins.Params[0].(int)
		nresults := ins.Params[1].(int)
		v.doCall(nargs, nresults)
	case bytecode.Return:
		count := ins.Params[0].(int)
		v.doReturn(f, count)

	// ----- numeric for -----
	case bytecode.ForPrep:
		baseSlot := ins.Params[0].(int)
		target := ins.Params[1].(int)
		v.forPrep(f, baseSlot, target)
	case bytecode.ForLoop:
		baseSlot := ins.Params[0].(int)
		target := ins.Params[1].(int)
		v.forLoop(f, baseSlot, target)

	// ----- generic for -----
	case bytecode.TForCall:
		baseSlot := ins.Params[0].(int)
		nresults := ins.Params[1].(int)
		v.tForCall(f, baseSlot, nresults)
	case bytecode.TForLoop:
		baseSlot := ins.Params[0].(int)
		target := ins.Params[1].(int)
		v.tForLoop(f, baseSlot, target)

	// ----- stack utility -----
	case bytecode.Pop:
		count := ins.Params[0].(int)
		v.popN(count)
	case bytecode.Dup:
		v.push(v.Stack[len(v.Stack)-1])

	// ----- frame -----
	case bytecode.Leave:
		// Implicit return-with-no-values for chunks that fall off the end.
		v.doReturn(f, 0)

	default:
		panic(fmt.Sprintf("vm: unknown opcode %d (%s)", ins.Opcode, bytecode.InstructionNameTable[ins.Opcode]))
	}
}

// ---------------------------------------------------------------------------
// Closure construction
// ---------------------------------------------------------------------------

// makeClosure builds a closure for `proto` capturing upvalues per its
// UpvalueDesc table. InStack=true descriptors look up the immediately
// enclosing function's stack slot; InStack=false descriptors share an
// upvalue from the enclosing closure.
func (v *VM) makeClosure(parent *CallFrame, proto *bytecode.InstructionSet) *Closure {
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

// ---------------------------------------------------------------------------
// Calls and returns
// ---------------------------------------------------------------------------

// doCall pops the function + nargs args and invokes it. The function lives
// at Stack[sp-nargs-1]; arguments at Stack[sp-nargs..sp]. Results land on
// the stack adjusted to nresults.
func (v *VM) doCall(nargs, nresults int) {
	if nargs < 0 {
		// "all from top" — we don't currently emit Call with -1 nargs; the
		// codegen always specifies nargs explicitly.
		panic("vm: Call with -1 nargs is unsupported")
	}
	argsStart := len(v.Stack) - nargs
	fn := v.Stack[argsStart-1]
	args := append([]Value(nil), v.Stack[argsStart:argsStart+nargs]...)
	v.Stack = v.Stack[:argsStart-1] // remove function and args from the stack

	switch g := fn.(type) {
	case *Closure:
		// callClosure runs the callee to completion and leaves its produced
		// results on the stack in the (now-popped) function slot's place.
		v.callClosureWithResults(g, args, nresults)
	case *GoFunc:
		results := g.Fn(v, args)
		v.pushResults(results, nresults)
	default:
		// __call metamethod fallback: any non-function with a __call entry
		// in its metatable can be invoked, with the original target spliced
		// in as the first argument.
		if results, ok := v.callMMIfNotFunction(fn, args, nresults); ok {
			v.pushResults(results, nresults)
			return
		}
		panic(errorf("attempt to call a %s value", TypeName(fn)))
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
	// Collect return values from the top of the stack.
	var rets []Value
	if count < 0 {
		// -1: every value pushed past the locals area is a return value.
		startBase := f.Base + f.Closure.Proto.NumLocals
		rets = append(rets, v.Stack[startBase:]...)
	} else {
		// Top `count` stack entries are the return values.
		start := len(v.Stack) - count
		rets = append(rets, v.Stack[start:]...)
	}
	v.unwindFrame(f, rets...)
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
func (v *VM) unwindFrame(f *CallFrame, rets ...Value) {
	v.closeUpvaluesAbove(f.Base)
	v.Stack = v.Stack[:f.Base]
	v.frames = v.frames[:len(v.frames)-1]
	v.pushResults(rets, f.NResults)
}

// ---------------------------------------------------------------------------
// Numeric for
// ---------------------------------------------------------------------------

// forPrep validates the start/limit/step, decrements the index by one step
// (so the first ForLoop add lands on `start`), and jumps to the matching
// ForLoop instruction.
func (v *VM) forPrep(f *CallFrame, baseSlot, target int) {
	startV := *v.localAt(f, baseSlot)
	limitV := *v.localAt(f, baseSlot+1)
	stepV := *v.localAt(f, baseSlot+2)

	// Promote to a common subtype: if any of the three is a float, all are
	// floats; otherwise everything stays an integer.
	if isFloat(startV) || isFloat(limitV) || isFloat(stepV) {
		s, ok1 := ToFloat(startV)
		l, ok2 := ToFloat(limitV)
		st, ok3 := ToFloat(stepV)
		if !ok1 || !ok2 || !ok3 {
			panic(luaError("'for' initial value, limit, and step must be numbers"))
		}
		if st == 0 {
			panic(luaError("'for' step is zero"))
		}
		*v.localAt(f, baseSlot) = s - st
		*v.localAt(f, baseSlot+1) = l
		*v.localAt(f, baseSlot+2) = st
	} else {
		s, ok1 := ToInteger(startV)
		l, ok2 := ToInteger(limitV)
		st, ok3 := ToInteger(stepV)
		if !ok1 || !ok2 || !ok3 {
			panic(luaError("'for' initial value, limit, and step must be numbers"))
		}
		if st == 0 {
			panic(luaError("'for' step is zero"))
		}
		*v.localAt(f, baseSlot) = s - st
		*v.localAt(f, baseSlot+1) = l
		*v.localAt(f, baseSlot+2) = st
	}
	f.IP = target
}

// forLoop adds step to the index; if the new value is still within [start,
// limit] (interpreted by the sign of step) it stores the index and jumps to
// the loop body, otherwise falls through.
func (v *VM) forLoop(f *CallFrame, baseSlot, target int) {
	idx := *v.localAt(f, baseSlot)
	limit := *v.localAt(f, baseSlot+1)
	step := *v.localAt(f, baseSlot+2)

	if isFloat(idx) || isFloat(limit) || isFloat(step) {
		i, _ := ToFloat(idx)
		l, _ := ToFloat(limit)
		s, _ := ToFloat(step)
		i += s
		if (s > 0 && i <= l) || (s < 0 && i >= l) {
			*v.localAt(f, baseSlot) = i
			f.IP = target
		}
		return
	}
	i, _ := ToInteger(idx)
	l, _ := ToInteger(limit)
	s, _ := ToInteger(step)
	i += s
	if (s > 0 && i <= l) || (s < 0 && i >= l) {
		*v.localAt(f, baseSlot) = i
		f.IP = target
	}
}

func isFloat(v Value) bool { _, ok := v.(float64); return ok }

// ---------------------------------------------------------------------------
// Generic for
// ---------------------------------------------------------------------------

// tForCall calls iter(state, control), placing nresults values into the
// visible-variable slots starting at baseSlot+3.
func (v *VM) tForCall(f *CallFrame, baseSlot, nresults int) {
	iter := *v.localAt(f, baseSlot)
	state := *v.localAt(f, baseSlot+1)
	control := *v.localAt(f, baseSlot+2)

	var results []Value
	switch g := iter.(type) {
	case *Closure:
		// Invoke as if `iter(state, control)` with nresults expected.
		base := len(v.Stack)
		v.callClosure(g, []Value{state, control}, nresults)
		// Results are now at the previous top.
		results = append(results, v.Stack[base:]...)
		v.Stack = v.Stack[:base]
	case *GoFunc:
		raw := g.Fn(v, []Value{state, control})
		for i := 0; i < nresults; i++ {
			if i < len(raw) {
				results = append(results, raw[i])
			} else {
				results = append(results, nil)
			}
		}
	default:
		panic(errorf("attempt to call a %s value (for iterator)", TypeName(iter)))
	}

	for i := 0; i < nresults; i++ {
		var val Value
		if i < len(results) {
			val = results[i]
		}
		*v.localAt(f, baseSlot+3+i) = val
	}
}

// tForLoop checks the first visible variable: if non-nil, advance the
// control slot to that value and jump to the body; otherwise fall through.
func (v *VM) tForLoop(f *CallFrame, baseSlot, target int) {
	first := *v.localAt(f, baseSlot+3)
	if first == nil {
		return
	}
	*v.localAt(f, baseSlot+2) = first
	f.IP = target
}

// (Table indexing now goes through indexMM / newIndexMM in metatable.go.)

// SetGlobal is a convenience helper for embedding code: register `name` in
// the globals table without going through bytecode.
func (v *VM) SetGlobal(name string, val Value) {
	v.Globals.Set(name, val)
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
	panic(errorf("attempt to call a %s value", TypeName(fn)))
}
