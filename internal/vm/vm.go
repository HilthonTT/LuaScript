package vm

import (
	"fmt"
	"math"
	"strings"

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

// recoverToError is the shared `defer`-installed recover for the
// top-level Run paths. A LuaError (script-raised) or a Go error
// surfaces as-is; anything else is wrapped with a "vm panic" prefix so
// callers can still log without losing the original value.
func (v *VM) recoverToError(err *error) {
	if r := recover(); r != nil {
		switch e := r.(type) {
		case luaError:
			// A script error(value) that reached the top level uncaught:
			// render it for display, honouring __tostring on table values.
			*err = LuaError(ToStringMM(v, e.value))
		case LuaError:
			*err = e
		case error:
			*err = e
		default:
			*err = fmt.Errorf("vm panic: %v", r)
		}
	}
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
	// fall back to a one-time scan of the instruction stream. The result is
	// cached on the proto via localsResolved — without that flag this scan
	// would run on every single call (a full instruction-stream walk).
	if !cl.Proto.LocalsResolved() {
		if n := computeMaxLocalSlots(cl.Proto); n > cl.Proto.NumLocals {
			cl.Proto.NumLocals = n
		}
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
			bump(int(ins.A))
		case bytecode.ForPrep, bytecode.ForLoop:
			bump(int(ins.A) + 2)
		case bytecode.TForCall:
			bump(int(ins.A) + 2 + int(ins.B))
		case bytecode.TForLoop:
			bump(int(ins.A) + 3)
		}
	}
	return maxSlot + 1
}

func (v *VM) push(x Value) {
	v.Stack = append(v.Stack, x)
}

// pushNils extends the stack by n nil slots in a single append, avoiding
// the per-iteration regrow that `for i := 0; i < n; i++ { v.push(nil) }`
// would pay for large n (LoadNil / LoadVararg padding). When the backing
// array already has capacity we reslice and explicitly clear, because
// pop/popN do not zero — stale Values would otherwise leak through.
func (v *VM) pushNils(n int) {
	if n <= 0 {
		return
	}
	cur := len(v.Stack)
	need := cur + n
	if cap(v.Stack) < need {
		v.Stack = append(v.Stack, make([]Value, n)...)
		return
	}
	v.Stack = v.Stack[:need]
	clear(v.Stack[cur:need])
}

func (v *VM) pop() Value {
	n := len(v.Stack) - 1
	x := v.Stack[n]
	v.Stack = v.Stack[:n]
	return x
}

// peek2 returns the top two stack values without changing the stack length:
// `a` is the lower (left) operand, `b` the upper (right). Paired with
// setTop2, it lets a binary opcode read both operands, compute, and replace
// them with a single result using one slice reslice instead of the two pops
// plus one push the naive form pays. The operands stay live on the stack
// across any metamethod call the slow path makes, which keeps them reachable
// for GC and is why the truncate happens afterwards in setTop2.
func (v *VM) peek2() (a, b Value) {
	n := len(v.Stack)
	return v.Stack[n-2], v.Stack[n-1]
}

// setTop2 replaces the top two stack values with a single result `r`,
// shrinking the stack by one. It is the write half of the peek2/setTop2 pair.
func (v *VM) setTop2(r Value) {
	n := len(v.Stack)
	v.Stack[n-2] = r
	v.Stack = v.Stack[:n-1]
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

// framePoolMax caps the per-VM recycle pool. Long-running coroutine-
// heavy programs would otherwise grow framePool without bound (release
// always appended) — frames beyond this count are dropped on the floor
// and let GC reclaim them. 256 covers normal recursion depths cheaply.
const framePoolMax = 256

// acquireFrame returns a CallFrame for a new activation, drawing from the
// recycle pool when one is available. Every field is overwritten so a
// recycled frame carries no state from its prior use.
func (v *VM) acquireFrame(cl *Closure, base, top, nresults int, varargs []Value) *CallFrame {
	n := len(v.framePool)
	if n == 0 {
		return &CallFrame{Closure: cl, Base: base, Top: top, NResults: nresults, Varargs: varargs}
	}
	f := v.framePool[n-1]
	v.framePool = v.framePool[:n-1]
	f.Closure = cl
	f.IP = 0
	f.Base = base
	f.Top = top
	f.NResults = nresults
	f.Varargs = varargs
	f.Deferred = nil
	return f
}

// releaseFrame returns a fully-unwound frame to the recycle pool. The frame
// must already be off v.frames and unreferenced; pointer fields are cleared
// so the pooled frame does not pin a closure or varargs slice alive.
// Beyond framePoolMax the frame is dropped so the pool stays bounded.
func (v *VM) releaseFrame(f *CallFrame) {
	if len(v.framePool) >= framePoolMax {
		return
	}
	f.Closure = nil
	f.Varargs = nil
	f.Deferred = nil
	v.framePool = append(v.framePool, f)
}

// findOrCreateOpenUpvalue returns the open upvalue tracking stack[index],
// creating one if none exists. Open upvalues are kept in v.openUpvs sorted
// by ascending Index — this function maintains that invariant by inserting
// at the binary-search position on a miss, which keeps lookups O(log n)
// instead of the linear scan a closure-heavy program would otherwise pay.
func (v *VM) findOrCreateOpenUpvalue(index int) *Upvalue {
	lo, hi := 0, len(v.openUpvs)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		u := v.openUpvs[mid]
		switch {
		case u.Index < index:
			lo = mid + 1
		case u.Index > index:
			hi = mid
		default:
			return u
		}
	}
	// Miss: insert a new upvalue at `lo` to preserve the sort order.
	u := &Upvalue{Stack: &v.Stack, Index: index}
	v.openUpvs = append(v.openUpvs, nil)
	copy(v.openUpvs[lo+1:], v.openUpvs[lo:len(v.openUpvs)-1])
	v.openUpvs[lo] = u
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
		valuesStart := len(v.Stack) - count
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

// ---------------------------------------------------------------------------
// Calls and returns
// ---------------------------------------------------------------------------

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

// forPrep validates the start/limit/step and stores the starting value in the
// index slot. If the loop body should run at least once it falls through into
// the body (the next instruction); otherwise it jumps past ForLoop to the exit
// target. Storing the real start — rather than start-step "undone" by the first
// ForLoop add — keeps the integer path free of the overflow that made loops
// near math.maxinteger run forever.
func (v *VM) forPrep(f *CallFrame, baseSlot, exitTarget int) {
	startV := *v.localAt(f, baseSlot)
	limitV := *v.localAt(f, baseSlot+1)
	stepV := *v.localAt(f, baseSlot+2)

	// Lua 5.4 §3.3.5: the loop runs with integers iff the initial value and
	// the step are both integers — the limit may be a float without forcing a
	// float loop (it is floored/ceiled to an integer bound instead).
	if isFloat(startV) || isFloat(stepV) {
		s, ok1 := ToFloat(startV)
		l, ok2 := ToFloat(limitV)
		st, ok3 := ToFloat(stepV)
		if !ok1 || !ok2 || !ok3 {
			panic(LuaError("'for' initial value, limit, and step must be numbers"))
		}
		if st == 0 {
			panic(LuaError("'for' step is zero"))
		}
		// Positive (s<=l) test, so a NaN limit yields zero iterations.
		if !((st > 0 && s <= l) || (st < 0 && s >= l)) {
			f.IP = exitTarget
			return
		}
		*v.localAt(f, baseSlot) = s
		*v.localAt(f, baseSlot+1) = l
		*v.localAt(f, baseSlot+2) = st
		return
	}
	s, ok1 := ToInteger(startV)
	st, ok3 := ToInteger(stepV)
	if !ok1 || !ok3 {
		panic(LuaError("'for' initial value and step must be numbers"))
	}
	if !isNumber(limitV) {
		panic(LuaError("'for' limit must be a number"))
	}
	if st == 0 {
		panic(LuaError("'for' step is zero"))
	}
	// The limit may be a float; convert it to the integer bound the loop will
	// actually compare against (floor for an ascending loop, ceil for a
	// descending one), keeping the loop variable an integer.
	l, run := forLimitInt(s, limitV, st)
	if !run {
		f.IP = exitTarget
		return
	}
	*v.localAt(f, baseSlot) = internInt(s)
	*v.localAt(f, baseSlot+1) = internInt(l)
	*v.localAt(f, baseSlot+2) = internInt(st)
}

// forLimitInt resolves an integer `for` loop's limit (which may be a float) to
// the integer bound the loop compares against, mirroring Lua 5.4's forlimit.
// It returns the bound and whether the loop should run at all. A float limit is
// floored for an ascending loop and ceiled for a descending one; out-of-range
// or NaN limits clamp to MaxInt64/MinInt64 or skip the loop entirely.
func forLimitInt(init int64, limitV Value, step int64) (int64, bool) {
	if i, ok := limitV.(int64); ok {
		return i, (step > 0 && init <= i) || (step < 0 && init >= i)
	}
	f, _ := ToFloat(limitV)
	if math.IsNaN(f) {
		return 0, false
	}
	var ff float64
	if step < 0 {
		ff = math.Ceil(f)
	} else {
		ff = math.Floor(f)
	}
	const twoPow63 = 9223372036854775808.0 // 2^63; the float just past MaxInt64
	var p int64
	switch {
	case ff >= -twoPow63 && ff < twoPow63:
		p = int64(ff)
	case f > 0:
		if step < 0 {
			return 0, false // limit far above any reachable value
		}
		p = math.MaxInt64
	default:
		if step > 0 {
			return 0, false // limit far below any reachable value
		}
		p = math.MinInt64
	}
	return p, (step > 0 && init <= p) || (step < 0 && init >= p)
}

func isNumber(v Value) bool {
	switch v.(type) {
	case int64, float64:
		return true
	}
	return false
}

// forLoop adds step to the index; if the new value is still within range
// (interpreted by the sign of step) it stores the index and jumps to the loop
// body, otherwise falls through to exit. The integer path detects signed
// overflow of the increment — a wrap means the loop has stepped past the
// representable range and must terminate, never spuriously re-enter.
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
	ni := i + s
	if s > 0 {
		// ni >= i ⟺ the add didn't overflow past math.maxinteger.
		if ni >= i && ni <= l {
			*v.localAt(f, baseSlot) = internInt(ni)
			f.IP = target
		}
	} else {
		// ni <= i ⟺ the add didn't underflow past math.mininteger.
		if ni <= i && ni >= l {
			*v.localAt(f, baseSlot) = internInt(ni)
			f.IP = target
		}
	}
}

func isFloat(v Value) bool { _, ok := v.(float64); return ok }

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
		panic(Errorf("attempt to call a %s value (for iterator)", TypeName(iter)))
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
