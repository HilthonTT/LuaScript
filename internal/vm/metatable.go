package vm

// Metatable support — every operator that accepts a metamethod fallback
// routes through one of the helpers below. The raw paths still live in
// arith.go / vm.go; these wrappers prefer the raw path and only consult the
// metatable when the raw path would have failed.
//
// The events list and dispatch order follow Lua 5.4 §2.4 (Metatables and
// Metamethods).

// metaEvent names every supported metamethod string. Stored as constants so
// the call sites read clearly.
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

// getMetamethod returns mt[event] for the metatable applicable to val, or
// nil if no metamethod is set. Tables carry their own metatable; strings
// share the VM's stringMeta; other primitive types currently have no
// metatable (would need typed metatables to enable).
func (v *VM) getMetamethod(val Value, event string) Value {
	mt := v.metatableOf(val)
	if mt == nil {
		return nil
	}
	return mt.Get(event)
}

// metatableOf returns the metatable for val, or nil if none.
func (v *VM) metatableOf(val Value) *Table {
	switch x := val.(type) {
	case *Table:
		return x.metatable
	case string:
		return v.stringMeta
	}
	return nil
}

// callMM invokes a metamethod, returning its single result. Most metamethods
// are defined to produce one value; the multi-value cases (e.g. iterating
// __pairs results) are handled separately at their call site.
func (v *VM) callMM(fn Value, args ...Value) Value {
	results := v.CallValue(fn, args, 1)
	if len(results) == 0 {
		return nil
	}
	return results[0]
}

// Arithmetic & bitwise dispatch

// arithMM implements the binary arithmetic dispatch: try the raw path, and
// only consult __<event> when the operands are not coercible to numbers.
// `op` is the operator string used by the existing intArith/floatArith
// helpers in integer.go / float.go.
func (v *VM) arithMM(a, b Value, op, event string) Value {
	// Fast path: operands already the same numeric subtype — the common
	// case. Skips arithMM's coercibility probe AND the second ToNumber pass
	// inside arith(). Mixed int/float and string-coercible operands fall
	// through to the general path, which still handles promotion.
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

// arithDivMM handles `/`, which is float-only at the raw level.
func (v *VM) arithDivMM(a, b Value) Value {
	// Fast path: both operands are numbers — divide directly instead of
	// running each through ToFloat twice (once to probe, once in arithDiv).
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

// arithFloorDivMM handles `//`.
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

// arithPowMM handles `^`.
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

// arithNegMM handles unary `-`.
func (v *VM) arithNegMM(a Value) Value {
	if _, _, _, ok := ToNumber(a); ok {
		return arithNeg(a)
	}
	if mm := v.getMetamethod(a, metaUnm); mm != nil {
		return v.callMM(mm, a, a)
	}
	panic(Errorf("attempt to perform arithmetic on a %s value", TypeName(a)))
}

// bitwiseMM handles & | ~ << >> with metamethod fallback.
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

// Concat / Length / Comparison

// concatMM joins two values with __concat fallback. Used for the n-ary
// Concat opcode by reducing pairwise from the right.
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

// lenMM implements `#` with __len fallback. Strings always return byte
// length; tables consult __len if present, otherwise the raw border length.
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

// EqualMM compares two values exactly as the `==` operator does, __eq
// metamethod included. Hosts that need script-visible equality — rather than
// the raw structural Equal — should use this.
func (v *VM) EqualMM(a, b Value) bool { return v.equalMM(a, b) }

// equalMM extends Equal with __eq fallback. Per Lua semantics __eq is only
// invoked when both operands are tables (or both are full userdata) and the
// raw == returned false.
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

// lessMM implements `<` with __lt fallback.
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

// lessOrEqualMM implements `<=` with __le fallback.
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

// Indexing / new-index / call

// indexMM reads obj[key] honoring __index. Reads the raw table first; if the
// key is absent and a metatable's __index is present, follows the chain
// (table __index recurses; function __index is called).
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
	// Non-table primary objects must consult __index from their metatable
	// (string is the common case).
	mm := v.getMetamethod(obj, metaIndex)
	if mm == nil {
		panic(Errorf("attempt to index a %s value", TypeName(obj)))
	}
	return v.indexViaMetamethod(obj, key, mm)
}

// maxMetaChain bounds how many links a __index / __newindex table chain may
// have before we assume a cycle. A self-referential chain (m.__index = m) would
// otherwise recurse forever and blow the Go stack with a fatal, pcall-uncatchable
// overflow. Matches Lua 5.4's MAXTAGLOOP; hitting it raises a catchable error.
const maxMetaChain = 2000

func (v *VM) indexViaMetamethod(obj, key, mm Value) Value {
	// Walk the __index table chain iteratively so a cyclic chain is caught by
	// the depth bound instead of overflowing the Go stack.
	for depth := 0; ; depth++ {
		m, ok := mm.(*Table)
		if !ok {
			// Function __index: call mm(obj, key); take first result.
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

// newIndexMM writes obj[key]=val honoring __newindex. If a __newindex is
// present and the key was absent in the raw table, the metamethod is invoked
// and the raw write does NOT happen (Lua spec). If the key was already
// present, the raw write happens directly.
func (v *VM) newIndexMM(obj, key, val Value) {
	// Walk the __newindex table chain iteratively; a cyclic chain is caught by
	// the depth bound rather than overflowing the Go stack.
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

// callMMIfNotFunction handles the __call metamethod. Returns (results, ok).
// When ok == false the caller should panic with the standard "attempt to
// call" message; that message lives in vm.go's doCall to keep it next to
// the regular call path.
func (v *VM) callMMIfNotFunction(fn Value, args []Value, nresults int) ([]Value, bool) {
	if mm := v.getMetamethod(fn, metaCall); mm != nil {
		// __call receives the original target as its first arg, then the
		// caller's args.
		full := append([]Value{fn}, args...)
		results := v.CallValue(mm, full, nresults)
		return results, true
	}
	return nil, false
}
