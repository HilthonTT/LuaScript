package vm

const (
	metaAdd      = "__add"
	metaSub      = "__sub"
	metaMul      = "__mul"
	metaDiv      = "__div"
	metaMod      = "__mod"
	metaPow      = "__pow"
	metaIDiv     = "__idiv"
	metaUnm      = "__unm"
	metaBAnd     = "__band"
	metaBOr      = "__bor"
	metaBXor     = "__bxor"
	metaShl      = "__shl"
	metaShr      = "__shr"
	metaBNot     = "__bnot"
	metaConcat   = "__concat"
	metaLen      = "__len"
	metaEq       = "__eq"
	metaLt       = "__lt"
	metaLe       = "__le"
	metaIndex    = "__index"
	metaNewIndex = "__newindex"
	metaCall     = "__call"
)

func (v *VM) getMetamethod(val Value, event string) Value {
	mt := v.metatableOf(val)
	if mt == nil {
		return nil
	}
	return mt.Get(event)
}

func (v *VM) metatableOf(val Value) *Table {
	switch x := val.(type) {
	case *Table:
		return x.metatable
	case string:
		return v.stringMeta
	}
	return nil
}

func (v *VM) callMM(fn Value, args ...Value) Value {
	results := v.CallValue(fn, args, 1)
	if len(results) == 0 {
		return nil
	}
	return results[0]
}

func (v *VM) arithMM(a, b Value, op, event string) Value {
	switch x := a.(type) {
	case int64:
		if y, ok := b.(int64); ok {
			return intArith(x, y, op)
		}
	case float64:
		if y, ok := b.(float64); ok {
			return floatArith(x, y, op)
		}
	}
	_, _, _, aOk := ToNumber(a)
	_, _, _, bOk := ToNumber(b)
	if aOk && bOk {
		return arith(a, b, op)
	}
	if mm := v.getMetamethod(a, event); mm != nil {
		return v.callMM(mm, a, b)
	}
	if mm := v.getMetamethod(b, event); mm != nil {
		return v.callMM(mm, a, b)
	}
	panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(firstNonNumber(a, b))))
}

func (v *VM) arithDivMM(a, b Value) Value {
	switch x := a.(type) {
	case int64:
		switch y := b.(type) {
		case int64:
			return float64(x) / float64(y)
		case float64:
			return float64(x) / y
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return x / float64(y)
		case float64:
			return x / y
		}
	}
	if _, aOk := ToFloat(a); aOk {
		if _, bOk := ToFloat(b); bOk {
			return arithDiv(a, b)
		}
	}
	if mm := v.getMetamethod(a, metaDiv); mm != nil {
		return v.callMM(mm, a, b)
	}
	if mm := v.getMetamethod(b, metaDiv); mm != nil {
		return v.callMM(mm, a, b)
	}
	panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(firstNonNumber(a, b))))
}

func (v *VM) arithFloorDivMM(a, b Value) Value {
	_, _, _, aOk := ToNumber(a)
	_, _, _, bOk := ToNumber(b)
	if aOk && bOk {
		return arithFloorDiv(a, b)
	}
	if mm := v.getMetamethod(a, metaIDiv); mm != nil {
		return v.callMM(mm, a, b)
	}
	if mm := v.getMetamethod(b, metaIDiv); mm != nil {
		return v.callMM(mm, a, b)
	}
	panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(firstNonNumber(a, b))))
}

func (v *VM) arithPowMM(a, b Value) Value {
	if _, aOk := ToFloat(a); aOk {
		if _, bOk := ToFloat(b); bOk {
			return arithPow(a, b)
		}
	}
	if mm := v.getMetamethod(a, metaPow); mm != nil {
		return v.callMM(mm, a, b)
	}
	if mm := v.getMetamethod(b, metaPow); mm != nil {
		return v.callMM(mm, a, b)
	}
	panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(firstNonNumber(a, b))))
}

func (v *VM) arithNegMM(a Value) Value {
	if _, _, _, ok := ToNumber(a); ok {
		return arithNeg(a)
	}
	if mm := v.getMetamethod(a, metaUnm); mm != nil {
		return v.callMM(mm, a, a)
	}
	panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(a)))
}

func (v *VM) bitwiseMM(a, b Value, raw func(a, b Value) Value, event string) Value {
	if _, aOk := ToInteger(a); aOk {
		if _, bOk := ToInteger(b); bOk {
			return raw(a, b)
		}
	}
	if mm := v.getMetamethod(a, event); mm != nil {
		return v.callMM(mm, a, b)
	}
	if mm := v.getMetamethod(b, event); mm != nil {
		return v.callMM(mm, a, b)
	}
	panic(Errorf("bitwise operand has no integer representation"))
}

func (v *VM) bitNotMM(a Value) Value {
	if _, ok := ToInteger(a); ok {
		return bitNot(a)
	}
	if mm := v.getMetamethod(a, metaBNot); mm != nil {
		return v.callMM(mm, a, a)
	}
	panic(Errorf("bitwise operand has no integer representation"))
}

func (v *VM) concatMM(a, b Value) Value {
	aok := isStringOrNumber(a)
	bok := isStringOrNumber(b)
	if aok && bok {
		return concatPair(a, b)
	}
	if mm := v.getMetamethod(a, metaConcat); mm != nil {
		return v.callMM(mm, a, b)
	}
	if mm := v.getMetamethod(b, metaConcat); mm != nil {
		return v.callMM(mm, a, b)
	}
	bad := a
	if aok {
		bad = b
	}
	panic(Errorf("attempt to concatenate a %s value", TypeName(bad)))
}

func isStringOrNumber(v Value) bool {
	switch v.(type) {
	case string, int64, float64:
		return true
	}
	return false
}

func (v *VM) lenMM(val Value) Value {
	if s, ok := val.(string); ok {
		return int64(len(s))
	}
	if mm := v.getMetamethod(val, metaLen); mm != nil {
		return v.callMM(mm, val)
	}
	if t, ok := val.(*Table); ok {
		return t.Len()
	}
	panic(Errorf("attempt to get length of a %s value", TypeName(val)))
}

func (v *VM) EqualMM(a, b Value) bool { return v.equalMM(a, b) }

func (v *VM) equalMM(a, b Value) bool {
	if Equal(a, b) {
		return true
	}
	ta, taOk := a.(*Table)
	tb, tbOk := b.(*Table)
	if !taOk || !tbOk {
		return false
	}
	if ta == tb {
		return true
	}
	if mm := v.getMetamethod(a, metaEq); mm != nil {
		return IsTruthy(v.callMM(mm, a, b))
	}
	if mm := v.getMetamethod(b, metaEq); mm != nil {
		return IsTruthy(v.callMM(mm, a, b))
	}
	return false
}

func (v *VM) lessMM(a, b Value) bool {
	if isOrderable(a, b) {
		return less(a, b)
	}
	if mm := v.getMetamethod(a, metaLt); mm != nil {
		return IsTruthy(v.callMM(mm, a, b))
	}
	if mm := v.getMetamethod(b, metaLt); mm != nil {
		return IsTruthy(v.callMM(mm, a, b))
	}
	panic(Errorf("attempt to compare %s with %s", TypeName(a), TypeName(b)))
}

func (v *VM) lessOrEqualMM(a, b Value) bool {
	if isOrderable(a, b) {
		return lessOrEqual(a, b)
	}
	if mm := v.getMetamethod(a, metaLe); mm != nil {
		return IsTruthy(v.callMM(mm, a, b))
	}
	if mm := v.getMetamethod(b, metaLe); mm != nil {
		return IsTruthy(v.callMM(mm, a, b))
	}
	panic(Errorf("attempt to compare %s with %s", TypeName(a), TypeName(b)))
}

func isOrderable(a, b Value) bool {
	switch a.(type) {
	case int64, float64:
		switch b.(type) {
		case int64, float64:
			return true
		}
	case string:
		_, ok := b.(string)
		return ok
	}
	return false
}

func (v *VM) indexMM(obj, key Value) Value {
	if t, ok := obj.(*Table); ok {
		raw := t.Get(key)
		if raw != nil {
			return raw
		}
		mm := v.getMetamethod(obj, metaIndex)
		if mm == nil {
			return nil
		}
		return v.indexViaMetamethod(obj, key, mm)
	}
	mm := v.getMetamethod(obj, metaIndex)
	if mm == nil {
		panic(Errorf("attempt to index a %s value", TypeName(obj)))
	}
	return v.indexViaMetamethod(obj, key, mm)
}

const maxMetaChain = 2000

func (v *VM) indexViaMetamethod(obj, key, mm Value) Value {
	for depth := 0; ; depth++ {
		m, ok := mm.(*Table)
		if !ok {
			return v.callMM(mm, obj, key)
		}
		raw := m.Get(key)
		if raw != nil {
			return raw
		}
		next := v.getMetamethod(m, metaIndex)
		if next == nil {
			return nil
		}
		if depth >= maxMetaChain {
			panic(LuaError("'__index' chain too long; possible loop"))
		}
		obj, mm = m, next
	}
}

func (v *VM) newIndexMM(obj, key, val Value) {
	for depth := 0; ; depth++ {
		if depth >= maxMetaChain {
			panic(LuaError("'__newindex' chain too long; possible loop"))
		}
		t, ok := obj.(*Table)
		if !ok {
			mm := v.getMetamethod(obj, metaNewIndex)
			if mm == nil {
				panic(Errorf("attempt to index a %s value", TypeName(obj)))
			}
			v.callMM(mm, obj, key, val)
			return
		}
		if t.Get(key) != nil {
			t.Set(key, val)
			return
		}
		mm := v.getMetamethod(obj, metaNewIndex)
		if mm == nil {
			t.Set(key, val)
			return
		}
		if m, isTable := mm.(*Table); isTable {
			obj = m
			continue
		}
		v.callMM(mm, obj, key, val)
		return
	}
}

func (v *VM) callMMIfNotFunction(fn Value, args []Value, nresults int) ([]Value, bool) {
	if mm := v.getMetamethod(fn, metaCall); mm != nil {
		full := append([]Value{fn}, args...)
		results := v.CallValue(mm, full, nresults)
		return results, true
	}
	return nil, false
}
