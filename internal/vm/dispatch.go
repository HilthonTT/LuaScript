package vm

// The interpreter core: exec, the protected (try) variant, and the opcode dispatch switch.

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/bytecode"
)

// exec runs the frame the caller just pushed. `entryDepth` is len(v.frames)
// immediately after that push, so the frame being run is frames[entryDepth-1]
// and exec returns once it has left.
//
// Bodies containing a `try` take the protected path, which installs a recover;
// everything else runs the bare loop below, so the overwhelming majority of
// calls pay nothing for the feature beyond one predictable branch.
//
// The loop is written out here rather than delegating to execLoop because exec
// is entered once per script call: routing the common path through a second
// non-inlinable call costs a few percent on call-heavy code (fib).
func (v *VM) exec(entryDepth int) {
	if v.frames[entryDepth-1].Closure.Proto.HasTry() {
		// Re-enter after every caught error: the handler has repointed the
		// frame's IP at its catch clause, and execLoop resumes from there.
		for v.execCatching(entryDepth) {
		}
		return
	}
	for len(v.frames) >= entryDepth {
		f := v.frames[len(v.frames)-1]
		if f.IP >= len(f.Closure.Proto.Instructions) {
			// Defensive — every well-formed proto ends with Leave/Return.
			v.unwindFrame(f, nil)
			continue
		}
		ins := f.Closure.Proto.Instructions[f.IP]
		f.IP++
		v.dispatch(f, ins)
	}
}

// execCatching runs the loop with a recover installed, returning true when an
// error was caught by a `try` in this exec's own frame and execution should
// resume at that frame's catch clause. Any error this frame cannot handle is
// re-panicked, so it keeps unwinding to an enclosing try, a pcall, or the host
// exactly as it would have.
func (v *VM) execCatching(entryDepth int) (resume bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		// coroutine.close's unwind sentinel is not a catchable error; a
		// `try` around a yield must not intercept the close.
		if isCloseSignal(r) {
			panic(r)
		}
		if !v.dispatchToHandler(entryDepth, r) {
			panic(r)
		}
		resume = true
	}()
	v.execLoop(entryDepth)
	return false
}

// dispatchToHandler unwinds an in-flight error to the innermost `try` open in
// frames[entryDepth-1], reporting whether it found one. It performs the same
// restoration safeCall does — run the abandoned frames' defers, close their
// open upvalues before the slots vanish, truncate frames/stack/callMarks — but
// stops at this frame instead of the pcall boundary, then leaves the error
// value on the stack for the catch clause to bind and repoints IP at it.
func (v *VM) dispatchToHandler(entryDepth int, r any) bool {
	if len(v.frames) < entryDepth {
		// Our own frame already returned; the error belongs to someone else.
		return false
	}
	f := v.frames[entryDepth-1]
	if len(f.handlers) == 0 {
		return false
	}
	h := f.handlers[len(f.handlers)-1]
	// Pop before running the catch clause, so an error raised inside the
	// handler propagates outward rather than re-entering the same handler.
	f.handlers = f.handlers[:len(f.handlers)-1]

	// Resolve the error value before the truncation below, while the frames
	// it was raised in are still live — that is what positions a VM-raised
	// error at its raise site rather than at the catch.
	caught := v.errorValue(r)

	// Deferred calls of every frame this unwind abandons, innermost first.
	// runDeferredSafely contains a panic from any single one so a faulty
	// cleanup can't replace the error being delivered. Done before the
	// truncation below, while those frames' locals are still live.
	for i := len(v.frames) - 1; i >= entryDepth; i-- {
		if len(v.frames[i].Deferred) > 0 {
			v.runDeferredSafely(v.frames[i])
		}
	}
	v.closeUpvaluesAbove(h.stackTop)
	v.frames = v.frames[:entryDepth]
	v.Stack = v.Stack[:h.stackTop]
	// An error thrown between a MarkArgs and its matching Call leaves pending
	// marks behind; drop them or the next variadic call in this frame pops a
	// stale mark and reads a bogus args base.
	v.callMarks = v.callMarks[:h.markDepth]

	f.IP = h.catchIP
	v.push(caught)
	return true
}

// execLoop drives the interpreter until the frame stack drops below
// `entryDepth` — i.e. until frames[entryDepth-1] returns. It is the body of
// exec's loop, used by the protected path; see the note on exec about why the
// unprotected path repeats it inline instead of calling here.
func (v *VM) execLoop(entryDepth int) {
	for len(v.frames) >= entryDepth {
		f := v.frames[len(v.frames)-1]
		if f.IP >= len(f.Closure.Proto.Instructions) {
			// Defensive — every well-formed proto ends with Leave/Return.
			v.unwindFrame(f, nil)
			continue
		}
		ins := f.Closure.Proto.Instructions[f.IP]
		f.IP++
		v.dispatch(f, ins)
	}
}

// dispatch executes a single instruction.
//
// All per-instruction parameters are read from the typed fast-path fields
// on bytecode.Instruction (A, B, StrA, BoxedAny). The dense iota switch
// over Opcode lets the Go compiler emit a jump table for the dispatch
// itself; the per-field reads avoid the type-assertion + slice bounds
// check that the older ins.Params[N].(T) layout paid on every dispatch.
func (v *VM) dispatch(f *CallFrame, ins *bytecode.Instruction) {
	switch ins.Opcode {

	// ----- constants & literals -----
	case bytecode.LoadNil:
		v.pushNils(int(ins.A))
	case bytecode.LoadTrue:
		v.push(true)
	case bytecode.LoadFalse:
		v.push(false)
	case bytecode.LoadInt:
		// BoxedAny carries the int64 already boxed in an interface so the
		// same box is shared by every execution of this instruction
		// instead of re-boxing (a 386 heap alloc) per push.
		v.push(ins.BoxedAny)
	case bytecode.LoadFloat:
		v.push(ins.BoxedAny)
	case bytecode.LoadString:
		v.push(ins.BoxedAny)
	case bytecode.LoadVararg:
		count := int(ins.A)
		va := f.Varargs
		switch {
		case count < 0:
			v.Stack = append(v.Stack, va...)
		case count <= len(va):
			v.Stack = append(v.Stack, va[:count]...)
		default:
			v.Stack = append(v.Stack, va...)
			v.pushNils(count - len(va))
		}
	case bytecode.Closure:
		proto := f.Closure.Proto.Protos[ins.A]
		v.push(v.makeClosure(f, proto))

	// ----- variables -----
	case bytecode.GetLocal:
		v.push(*v.localAt(f, int(ins.A)))
	case bytecode.SetLocal:
		*v.localAt(f, int(ins.A)) = v.pop()
	case bytecode.GetUpvalue:
		v.push(f.Closure.Upvalues[ins.A].Get())
	case bytecode.SetUpvalue:
		f.Closure.Upvalues[ins.A].Set(v.pop())
	case bytecode.GetGlobal:
		// Per-call-site monomorphic inline cache. Globals.gen bumps on
		// every Set / removeHashKey; on match we skip the string-hash +
		// map lookup entirely. On miss, do the lookup and store
		// (gen, val) for next time.
		if ins.CacheGen() == v.Globals.gen {
			v.push(ins.CacheVal())
		} else {
			val := v.Globals.Get(ins.StrA)
			ins.SetCache(v.Globals.gen, val)
			v.push(val)
		}
	case bytecode.SetGlobal:
		v.Globals.Set(ins.StrA, v.pop())

	// ----- tables -----
	case bytecode.NewTable:
		v.push(NewTable(int(ins.A), int(ins.B)))
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
		obj := v.pop()
		v.push(v.indexMM(obj, ins.StrA))
	case bytecode.SetField:
		val := v.pop()
		obj := v.pop()
		v.newIndexMM(obj, ins.StrA, val)
	case bytecode.Self:
		obj := v.Stack[len(v.Stack)-1] // peek; we want both method and obj
		v.Stack[len(v.Stack)-1] = v.indexMM(obj, ins.StrA)
		v.push(obj)
	case bytecode.SetList:
		count := int(ins.A)
		offset := int(ins.B)
		// Stack layout: [..., table, v1, v2, ..., vCount]
		var valuesStart int
		if count < 0 {
			// Variadic tail (e.g. {f()} / {...}): codegen emitted MarkArgs
			// before the spread values so their base is recoverable now.
			if len(v.callMarks) == 0 {
				panic("vm: SetList with count=-1 but no MarkArgs mark on stack")
			}
			valuesStart = v.callMarks[len(v.callMarks)-1]
			v.callMarks = v.callMarks[:len(v.callMarks)-1]
			count = len(v.Stack) - valuesStart
		} else {
			valuesStart = len(v.Stack) - count
		}
		t := v.Stack[valuesStart-1].(*Table)
		for i := 0; i < count; i++ {
			t.Set(int64(offset+i+1), v.Stack[valuesStart+i])
		}
		v.popN(count)

	// ----- arithmetic ----- (each routes through metatable.go for metamethod fallback)
	case bytecode.Add:
		a, b := v.peek2()
		// Hot path: same-type numeric operands skip the type-switch +
		// string-keyed dispatch in arithMM entirely. Wraparound semantics
		// match Lua 5.4 (signed two's-complement, no overflow check).
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				// internInt reuses a pre-boxed Value for small-magnitude
				// results (loop counters, indices, small accumulators), so
				// the common case avoids the per-op heap box that plain
				// `any(ai+bi)` would pay for any value above 255.
				v.setTop2(internInt(ai + bi))
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af + bf)
				return
			}
		}
		v.setTop2(v.arithMM(a, b, "+", metaAdd))
	case bytecode.Sub:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(internInt(ai - bi))
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af - bf)
				return
			}
		}
		v.setTop2(v.arithMM(a, b, "-", metaSub))
	case bytecode.Mul:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(internInt(ai * bi))
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af * bf)
				return
			}
		}
		v.setTop2(v.arithMM(a, b, "*", metaMul))
	case bytecode.Div:
		a, b := v.peek2()
		v.setTop2(v.arithDivMM(a, b))
	case bytecode.FloorDiv:
		a, b := v.peek2()
		v.setTop2(v.arithFloorDivMM(a, b))
	case bytecode.Mod:
		a, b := v.peek2()
		v.setTop2(v.arithMM(a, b, "%", metaMod))
	case bytecode.Pow:
		a, b := v.peek2()
		v.setTop2(v.arithPowMM(a, b))
	case bytecode.Neg:
		v.push(v.arithNegMM(v.pop()))

	// ----- bitwise -----
	case bytecode.BitAnd:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, bitAnd, metaBAnd))
	case bytecode.BitOr:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, bitOr, metaBOr))
	case bytecode.BitXor:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, bitXor, metaBXor))
	case bytecode.Shl:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, shl, metaShl))
	case bytecode.Shr:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, shr, metaShr))
	case bytecode.BitNot:
		v.push(v.bitNotMM(v.pop()))

	// ----- string / length -----
	case bytecode.Concat:
		count := int(ins.A)
		start := len(v.Stack) - count
		// Fast path: all operands are strings/numbers, so no __concat
		// metamethod can fire — build the result in one pass instead of
		// the quadratic pairwise reduction.
		allPlain := true
		size := 0
		for i := start; i < len(v.Stack); i++ {
			if s, ok := v.Stack[i].(string); ok {
				size += len(s)
				continue
			}
			if !isStringOrNumber(v.Stack[i]) {
				allPlain = false
				break
			}
			size += 16 // rough number-rendering estimate
		}
		if allPlain {
			var b strings.Builder
			b.Grow(size)
			for i := start; i < len(v.Stack); i++ {
				b.WriteString(concatOne(v.Stack[i]))
			}
			v.popN(count)
			v.push(b.String())
			return
		}
		// Right-associative: reduce pairwise from the right so __concat
		// metamethods see the same operand pairing as a chained `..`.
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
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai == bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af == bf)
				return
			}
		}
		if as, ok := a.(string); ok {
			if bs, ok := b.(string); ok {
				v.setTop2(as == bs)
				return
			}
		}
		v.setTop2(v.equalMM(a, b))
	case bytecode.NotEq:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai != bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af != bf)
				return
			}
		}
		if as, ok := a.(string); ok {
			if bs, ok := b.(string); ok {
				v.setTop2(as != bs)
				return
			}
		}
		v.setTop2(!v.equalMM(a, b))
	case bytecode.Lt:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai < bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af < bf)
				return
			}
		}
		v.setTop2(v.lessMM(a, b))
	case bytecode.Le:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai <= bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af <= bf)
				return
			}
		}
		v.setTop2(v.lessOrEqualMM(a, b))
	case bytecode.Gt:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai > bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af > bf)
				return
			}
		}
		v.setTop2(v.lessMM(b, a))
	case bytecode.Ge:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai >= bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af >= bf)
				return
			}
		}
		v.setTop2(v.lessOrEqualMM(b, a))

	// ----- logical -----
	case bytecode.Not:
		v.push(!IsTruthy(v.pop()))

	// ----- control flow -----
	case bytecode.Jump:
		f.IP = int(ins.A)
	case bytecode.JumpIfFalse:
		x := v.pop()
		if !IsTruthy(x) {
			f.IP = int(ins.A)
		}
	case bytecode.JumpIfTrue:
		x := v.pop()
		if IsTruthy(x) {
			f.IP = int(ins.A)
		}
	case bytecode.JumpIfFalseKeep:
		x := v.Stack[len(v.Stack)-1]
		if !IsTruthy(x) {
			f.IP = int(ins.A)
		} else {
			v.pop()
		}
	case bytecode.JumpIfTrueKeep:
		x := v.Stack[len(v.Stack)-1]
		if IsTruthy(x) {
			f.IP = int(ins.A)
		} else {
			v.pop()
		}

	// ----- calls / returns -----
	case bytecode.MarkArgs:
		v.callMarks = append(v.callMarks, len(v.Stack))
	case bytecode.CloseUpvalues:
		// End-of-iteration close so each loop turn captures fresh variables.
		v.closeUpvaluesAbove(f.Base + int(ins.A))
	case bytecode.Call:
		v.doCall(int(ins.A), int(ins.B))
	case bytecode.Return:
		v.doReturn(f, int(ins.A))

	// ----- numeric for -----
	case bytecode.ForPrep:
		v.forPrep(f, int(ins.A), int(ins.B))
	case bytecode.ForLoop:
		v.forLoop(f, int(ins.A), int(ins.B))

	// ----- generic for -----
	case bytecode.TForCall:
		v.tForCall(f, int(ins.A), int(ins.B))
	case bytecode.TForLoop:
		v.tForLoop(f, int(ins.A), int(ins.B))

	// ----- stack utility -----
	case bytecode.Pop:
		v.popN(int(ins.A))
	case bytecode.Dup:
		v.push(v.Stack[len(v.Stack)-1])

	// ----- frame -----
	case bytecode.Leave:
		// Implicit return-with-no-values for chunks that fall off the end.
		v.doReturn(f, 0)
	case bytecode.Defer:
		// Top of stack is the zero-arg closure the generator built for this
		// `defer`. Register it on the frame; it runs when the frame unwinds.
		d := v.pop()
		cl, ok := d.(*Closure)
		if !ok {
			panic(Errorf("defer: expected a function, got %s", TypeName(d)))
		}
		f.Deferred = append(f.Deferred, cl)

	case bytecode.Try:
		// Open a protected region. The stack is at a statement boundary here,
		// so its height is exactly what the catch clause should see restored.
		f.handlers = append(f.handlers, tryHandler{
			catchIP:   int(ins.A),
			stackTop:  len(v.Stack),
			markDepth: len(v.callMarks),
		})
	case bytecode.EndTry:
		// Left a protected region without raising — drop its handler(s).
		f.handlers = f.handlers[:len(f.handlers)-int(ins.A)]
	case bytecode.Throw:
		panic(luaError{value: v.pop()})

	default:
		panic(fmt.Sprintf("vm: unknown opcode %d (%s)", ins.Opcode, bytecode.InstructionNameTable[ins.Opcode]))
	}
}
