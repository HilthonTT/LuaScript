package vm

import "math"

func (v *VM) forPrep(f *CallFrame, baseSlot, exitTarget int) {
	startV := *v.localAt(f, baseSlot)
	limitV := *v.localAt(f, baseSlot+1)
	stepV := *v.localAt(f, baseSlot+2)

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
		if !((st > 0 && s <= l) || (st < 0 && s >= l)) {
			f.IP = exitTarget
			return
		}
		*v.localAt(f, baseSlot) = s
		*v.localAt(f, baseSlot+1) = l
		*v.localAt(f, baseSlot+2) = st
		*v.localAt(f, baseSlot+3) = s
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
	l, run := forLimitInt(s, limitV, st)
	if !run {
		f.IP = exitTarget
		return
	}
	sv := internInt(s)
	*v.localAt(f, baseSlot) = sv
	*v.localAt(f, baseSlot+1) = internInt(l)
	*v.localAt(f, baseSlot+2) = internInt(st)
	*v.localAt(f, baseSlot+3) = sv
}

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
	const twoPow63 = 9223372036854775808.0
	var p int64
	switch {
	case ff >= -twoPow63 && ff < twoPow63:
		p = int64(ff)
	case f > 0:
		if step < 0 {
			return 0, false
		}
		p = math.MaxInt64
	default:
		if step > 0 {
			return 0, false
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
			*v.localAt(f, baseSlot+3) = i
			f.IP = target
		}
		return
	}
	i, _ := ToInteger(idx)
	l, _ := ToInteger(limit)
	s, _ := ToInteger(step)
	ni := i + s
	if s > 0 {
		if ni >= i && ni <= l {
			nv := internInt(ni)
			*v.localAt(f, baseSlot) = nv
			*v.localAt(f, baseSlot+3) = nv
			f.IP = target
		}
	} else {
		if ni <= i && ni >= l {
			nv := internInt(ni)
			*v.localAt(f, baseSlot) = nv
			*v.localAt(f, baseSlot+3) = nv
			f.IP = target
		}
	}
}

func isFloat(v Value) bool { _, ok := v.(float64); return ok }

func (v *VM) tForCall(f *CallFrame, baseSlot, nresults int) {
	iter := *v.localAt(f, baseSlot)
	state := *v.localAt(f, baseSlot+1)
	control := *v.localAt(f, baseSlot+2)

	var results []Value
	switch g := iter.(type) {
	case *Closure:
		base := len(v.Stack)
		v.callClosure(g, []Value{state, control}, nresults)
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

func (v *VM) tForLoop(f *CallFrame, baseSlot, target int) {
	first := *v.localAt(f, baseSlot+3)
	if first == nil {
		return
	}
	*v.localAt(f, baseSlot+2) = first
	f.IP = target
}
