package vm

func (v *VM) push(x Value) {
	v.Stack = append(v.Stack, x)
}

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

func (v *VM) peek2() (a, b Value) {
	n := len(v.Stack)
	return v.Stack[n-2], v.Stack[n-1]
}

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

func (v *VM) localAt(f *CallFrame, i int) *Value {
	return &v.Stack[f.Base+i]
}

const framePoolMax = 256

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
	f.tbc = nil
	f.handlers = f.handlers[:0]
	return f
}

func (v *VM) releaseFrame(f *CallFrame) {
	if len(v.framePool) >= framePoolMax {
		return
	}
	f.Closure = nil
	f.Varargs = nil
	f.Deferred = nil
	f.tbc = nil
	v.framePool = append(v.framePool, f)
}

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
	u := &Upvalue{Stack: &v.Stack, Index: index}
	v.openUpvs = append(v.openUpvs, nil)
	copy(v.openUpvs[lo+1:], v.openUpvs[lo:len(v.openUpvs)-1])
	v.openUpvs[lo] = u
	return u
}

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
