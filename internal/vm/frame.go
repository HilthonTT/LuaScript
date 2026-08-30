package vm

// Frame plumbing: value-stack primitives, the call-frame pool, and open-upvalue bookkeeping.

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

// maxCallDepth bounds the number of nested Lua call frames; see calldepth.go
// and calldepth_race.go for the value and the reasoning behind it.

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
	// Reslice rather than nil out: tryHandler holds no pointers, so keeping
	// the backing array pins nothing and a `try`-heavy frame reuses it.
	f.handlers = f.handlers[:0]
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
