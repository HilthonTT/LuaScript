package vm

import "math"

type Table struct {
	array     []Value
	hash      map[Value]Value
	keys      []Value
	keyPos    map[Value]int
	dead      int
	metatable *Table

	gen uint32
}

func (t *Table) Metatable() *Table {
	return t.metatable
}

func (t *Table) SetMetatable(mt *Table) {
	t.metatable = mt
}

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

func (t *Table) Set(key, value Value) {
	if key == nil {
		panic(LuaError("table index is nil"))
	}
	if f, ok := key.(float64); ok && math.IsNaN(f) {
		panic(LuaError("table index is NaN"))
	}
	t.gen++
	if i, ok := arrayIndex(key); ok && i >= 1 {
		if i <= int64(len(t.array)) {
			t.array[i-1] = value
			if value == nil && i == int64(len(t.array)) {
				for len(t.array) > 0 && t.array[len(t.array)-1] == nil {
					t.array = t.array[:len(t.array)-1]
				}
			}
			return
		}
		if i == int64(len(t.array))+1 && value != nil {
			t.array = append(t.array, value)
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
	t.keys[idx] = nil
	t.dead++
}

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
		t.keys[i] = nil
	}
	t.keys = live
	t.keyPos = pos
	t.dead = 0
}

func (t *Table) Len() int64 {
	return int64(len(t.array))
}

func (t *Table) EntryCount() int64 {
	n := int64(len(t.hash))
	for _, v := range t.array {
		if v != nil {
			n++
		}
	}
	return n
}

func (t *Table) Append(v Value) {
	t.array = append(t.array, v)
}

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

func normalizeKey(v Value) Value {
	if f, ok := v.(float64); ok {
		if i, ok := floatToInt(f); ok {
			return i
		}
	}
	return v
}

func (t *Table) Next(key Value) (Value, Value) {
	if key == nil {
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
