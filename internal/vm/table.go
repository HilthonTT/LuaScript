package vm

import "math"

// Table is a Lua table, split into an array part keyed 1..n and a hash part
// for everything else. Hash iteration is insertion-ordered (Lua doesn't
// guarantee an order, but determinism makes pairs() predictable for tests).
//
// The metatable field, if non-nil, gives this table its metamethods (see
// vm/metatable.go). Use Metatable / SetMetatable for external access.
type Table struct {
	array     []Value
	hash      map[Value]Value
	keys      []Value       // hash keys in insertion order; nil entries are tombstones
	keyPos    map[Value]int // key -> index in keys; deleted keys keep their entry until compaction so Next can resume from them
	dead      int           // tombstone count in keys
	metatable *Table

	// gen is a monotonically-increasing generation counter bumped on
	// every Set / removeHashKey. The GetGlobal inline cache on
	// bytecode.Instruction stores the gen value observed at the last
	// lookup, so a single uint32 compare on the dispatch path replaces
	// a string-keyed map lookup whenever globals are stable across the
	// call site. Any mutation invalidates every cache slot at once;
	// the counter never decreases, so no ABA risk.
	gen uint32
}

// Metatable returns the table's metatable, or nil.
func (t *Table) Metatable() *Table {
	return t.metatable
}

// SetMetatable assigns mt as the metatable. Pass nil to clear.
func (t *Table) SetMetatable(mt *Table) {
	t.metatable = mt
}

// NewTable returns an empty table. The hints are used only when non-zero —
// a {} literal that ends up empty (a common pattern for "carrier" objects)
// stays at zero allocations. Set() and the array/hash growth paths handle
// nil-slice/map promotion lazily.
func NewTable(arrHint, hashHint int) *Table {
	t := &Table{}
	if arrHint > 0 {
		t.array = make([]Value, 0, arrHint)
	}
	if hashHint > 0 {
		t.hash = make(map[Value]Value, hashHint)
		t.keys = make([]Value, 0, hashHint)
		t.keyPos = make(map[Value]int, hashHint)
	}
	return t
}

// Get returns t[key]. Missing or nil keys yield nil.
func (t *Table) Get(key Value) Value {
	if key == nil {
		return nil
	}
	if i, ok := arrayIndex(key); ok && i >= 1 && i <= int64(len(t.array)) {
		return t.array[i-1]
	}
	if t.hash == nil {
		return nil
	}
	return t.hash[normalizeKey(key)]
}

// Set assigns t[key]=value. A nil value removes the binding. Setting a nil
// or NaN key panics with a Lua-style error.
func (t *Table) Set(key, value Value) {
	if key == nil {
		panic(LuaError("table index is nil"))
	}
	if f, ok := key.(float64); ok && math.IsNaN(f) {
		panic(LuaError("table index is NaN"))
	}
	// Bump the generation counter so any GetGlobal inline-cache slot
	// that snapshotted a prior gen will re-resolve on its next hit. The
	// bump is unconditional even on no-op writes (e.g. assigning the
	// same value); the cost is one increment vs. the correctness risk
	// of letting stale cached values survive a write.
	t.gen++
	if i, ok := arrayIndex(key); ok && i >= 1 {
		if i <= int64(len(t.array)) {
			t.array[i-1] = value
			if value == nil && i == int64(len(t.array)) {
				// Trim trailing nils so Len stays accurate.
				for len(t.array) > 0 && t.array[len(t.array)-1] == nil {
					t.array = t.array[:len(t.array)-1]
				}
			}
			return
		}
		if i == int64(len(t.array))+1 && value != nil {
			t.array = append(t.array, value)
			// Promote consecutive integer keys from the hash to the array.
			for {
				next := int64(len(t.array)) + 1
				if t.hash == nil {
					break
				}
				v, ok := t.hash[next]
				if !ok {
					break
				}
				t.array = append(t.array, v)
				t.removeHashKey(next)
			}
			return
		}
	}
	nk := normalizeKey(key)
	if value == nil {
		if t.hash != nil {
			if _, exists := t.hash[nk]; exists {
				t.removeHashKey(nk)
			}
		}
		return
	}
	if t.hash == nil {
		t.hash = make(map[Value]Value)
	}
	if _, exists := t.hash[nk]; !exists {
		if t.keyPos == nil {
			t.keyPos = make(map[Value]int)
		}
		// Compact only when inserting a new key, never on delete: Lua
		// allows removing the current field during pairs(), so positions
		// of deleted keys must survive until the next insertion (which is
		// undefined during traversal anyway, like Lua's rehash-on-insert).
		if t.dead > 8 && t.dead*2 >= len(t.keys) {
			t.compactKeys()
		}
		t.keyPos[nk] = len(t.keys)
		t.keys = append(t.keys, nk)
	}
	t.hash[nk] = value
}

func (t *Table) removeHashKey(k Value) {
	delete(t.hash, k)
	idx, ok := t.keyPos[k]
	if !ok {
		return
	}
	// Tombstone instead of splicing so removal is O(1) and an in-flight
	// Next() on the removed key can still resume from its old position.
	t.keys[idx] = nil
	t.dead++
}

// compactKeys rebuilds keys/keyPos without tombstones. Dropping the stale
// keyPos entries here is what lets removeHashKey keep them around cheaply.
func (t *Table) compactKeys() {
	live := t.keys[:0]
	pos := make(map[Value]int, len(t.hash))
	for _, k := range t.keys {
		if k != nil {
			pos[k] = len(live)
			live = append(live, k)
		}
	}
	for i := len(live); i < len(t.keys); i++ {
		t.keys[i] = nil // unpin dropped slots in the backing array
	}
	t.keys = live
	t.keyPos = pos
	t.dead = 0
}

// Len returns the table's array-part length. This is Lua's "border" length
// for the common case where the array part has no holes.
func (t *Table) Len() int64 {
	return int64(len(t.array))
}

// EntryCount returns the total number of key/value pairs stored in the table:
// the non-nil entries in the array part plus every live entry in the hash part.
// Unlike Len it counts past array holes and includes non-integer keys, so it
// reflects the table's actual occupancy rather than its Lua border length.
func (t *Table) EntryCount() int64 {
	n := int64(len(t.hash))
	for _, v := range t.array {
		if v != nil {
			n++
		}
	}
	return n
}

// Append pushes v onto the end of the array part. Used by SetList to bulk-fill.
func (t *Table) Append(v Value) {
	t.array = append(t.array, v)
}

// arrayIndex returns the integer index for v if v is integer-valued.
func arrayIndex(v Value) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case float64:
		if i, ok := floatToInt(x); ok {
			return i, true
		}
	}
	return 0, false
}

// normalizeKey collapses integer-valued floats into int64 so equal numeric
// keys hash the same regardless of how they were typed.
func normalizeKey(v Value) Value {
	if f, ok := v.(float64); ok {
		if i, ok := floatToInt(f); ok {
			return i
		}
	}
	return v
}

// Next returns the (k, v) pair that follows key in iteration order. Calling
// with key==nil starts iteration. When iteration is exhausted both returns
// are nil. Iteration first walks the array part, then the hash part in
// insertion order.
func (t *Table) Next(key Value) (Value, Value) {
	if key == nil {
		// Start with the first non-nil array entry.
		for i, v := range t.array {
			if v != nil {
				return int64(i + 1), v
			}
		}
		return t.firstHash()
	}

	if idx, ok := arrayIndex(key); ok && idx >= 1 && idx <= int64(len(t.array)) {
		for i := int(idx); i < len(t.array); i++ {
			if t.array[i] != nil {
				return int64(i + 1), t.array[i]
			}
		}
		return t.firstHash()
	}

	// Continue inside the hash part: jump to `key`'s recorded position and
	// return the next live entry. Deleted keys keep their keyPos entry until
	// compaction, so removing the current key mid-pairs() stays safe.
	nk := normalizeKey(key)
	if idx, ok := t.keyPos[nk]; ok {
		for i := idx + 1; i < len(t.keys); i++ {
			if k := t.keys[i]; k != nil {
				return k, t.hash[k]
			}
		}
	}
	return nil, nil
}

func (t *Table) firstHash() (Value, Value) {
	for _, k := range t.keys {
		if k != nil {
			return k, t.hash[k]
		}
	}
	return nil, nil
}
