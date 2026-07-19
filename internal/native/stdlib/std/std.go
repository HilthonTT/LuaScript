package std

import (
	"math"

	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterStdPreload installs the `std` module under package.preload.
// `require("std")` returns a table of constructors for the data
// structures defined in this package — stack, queue, deque, set,
// linked list, heap, hashmap, trie, btree. Each constructor returns a fresh object
// whose methods are bound to a private Go instance via colon-call.
func RegisterStdPreload(v *vm.VM) {
	vm.RegisterPreload(v, "std", stdLoader)
}

func stdLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newStdModule()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newStdModule() *vm.Table {
	m := vm.NewTable(0, 8)
	methods := vm.NewTable(0, 8)

	methods.Set("new_stack", &vm.GoFunc{Name: "std:new_stack", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapStack(NewStack())}
	}})
	methods.Set("new_queue", &vm.GoFunc{Name: "std:new_queue", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapQueue(NewQueue())}
	}})
	methods.Set("new_deque", &vm.GoFunc{Name: "std:new_deque", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapDeque(NewDeque())}
	}})
	methods.Set("new_set", &vm.GoFunc{Name: "std:new_set", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapSet(NewSet())}
	}})
	methods.Set("new_list", &vm.GoFunc{Name: "std:new_list", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapList(NewLinkedList())}
	}})
	methods.Set("new_hashmap", &vm.GoFunc{Name: "std:new_hashmap", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapHashMap(DefaultHashMap())}
	}})
	// std.new_heap(cmp) — cmp(a,b) must return truthy iff a should
	// sit *above* b on the heap (i.e. it's a less-than predicate, just
	// like sort.sort). Without a cmp we'd have no defensible default
	// ordering for arbitrary Lua values, so it's required.
	methods.Set("new_heap", &vm.GoFunc{Name: "std:new_heap", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 1 || args[0] == nil {
			panic(vm.Errorf("bad argument #1 to 'std.new_heap' (function expected, got nil)"))
		}
		cmp := args[0]
		switch cmp.(type) {
		case *vm.Closure, *vm.GoFunc:
		default:
			panic(vm.Errorf("bad argument #1 to 'std.new_heap' (function expected, got %s)", vm.TypeName(cmp)))
		}
		less := func(a, b any) bool {
			r := v.CallValue(cmp, []vm.Value{a, b}, 1)
			if len(r) == 0 {
				return false
			}
			return vm.IsTruthy(r[0])
		}
		h, _ := NewAny(less)
		return []vm.Value{wrapHeap(h)}
	}})
	// std.new_btree(max_keys?) — an ordered B-tree over number OR string
	// keys (the key type is locked in by the first insert; mixing raises).
	// max_keys is the per-node key capacity, default 3, minimum 3.
	methods.Set("new_btree", &vm.GoFunc{Name: "std:new_btree", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		maxKeys := int(vm.OptInt("std.new_btree", 1, args, 3))
		if maxKeys < 3 {
			panic(vm.Errorf("bad argument #1 to 'std.new_btree' (max_keys must be >= 3)"))
		}
		// Each node preallocates maxKeys keys + maxKeys+1 children, so an
		// absurd capacity would OOM on the first insert.
		if maxKeys > 1<<16 {
			panic(vm.Errorf("bad argument #1 to 'std.new_btree' (max_keys must be <= %d)", 1<<16))
		}
		return []vm.Value{wrapBTree(&btreeBox{maxKeys: maxKeys})}
	}})
	// std.new_trie() — a string-keyed prefix tree. insert/find/remove accept
	// one or more words; remove is lazy (marks non-leaf) so call compact() to
	// reclaim dead nodes. size() counts words, capacity() counts nodes.
	methods.Set("new_trie", &vm.GoFunc{Name: "std:new_trie", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{wrapTrie(NewNode())}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// wrapTrie exposes a *Node prefix tree as a Lua object. insert/find/remove
// take string words; non-string arguments raise the usual bad-argument error.
func wrapTrie(n *Node) *vm.Table {
	methods := vm.NewTable(0, 6)
	methods.Set("insert", &vm.GoFunc{Name: "trie:insert", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		for i := 2; i <= len(args); i++ {
			n.insert(vm.StringArg("trie:insert", i, args))
		}
		return nil
	}})
	methods.Set("find", &vm.GoFunc{Name: "trie:find", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{n.Find(vm.StringArg("trie:find", 2, args))}
	}})
	methods.Set("remove", &vm.GoFunc{Name: "trie:remove", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		for i := 2; i <= len(args); i++ {
			n.remove(vm.StringArg("trie:remove", i, args))
		}
		return nil
	}})
	methods.Set("compact", &vm.GoFunc{Name: "trie:compact", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		n.Compact()
		return nil
	}})
	methods.Set("size", &vm.GoFunc{Name: "trie:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(n.Size())}
	}})
	methods.Set("capacity", &vm.GoFunc{Name: "trie:capacity", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(n.Capacity())}
	}})
	return withMethods(methods)
}

// btreeBox routes Lua keys to a number- or string-keyed BTree. BTree is
// generic over constraints.Ordered, so a single tree can't hold arbitrary
// Lua values: the first insert picks the concrete tree, and every later
// key must be of the same type. Number keys are stored as float64, so
// integers beyond 2^53 lose precision.
type btreeBox struct {
	maxKeys int
	count   int
	nums    *BTree[float64]
	strs    *BTree[string]
}

func btreeKey(fname string, idx int, v vm.Value) (f float64, s string, isNum bool) {
	switch k := v.(type) {
	case int64:
		return float64(k), "", true
	case float64:
		// NaN compares false against everything, so it would break the
		// sorted invariant on insert and be unfindable/unremovable after —
		// silently corrupting the tree. Reject it like Lua rejects NaN
		// table keys.
		if math.IsNaN(k) {
			panic(vm.Errorf("bad argument #%d to '%s' (key is NaN)", idx, fname))
		}
		return k, "", true
	case string:
		return 0, k, false
	default:
		panic(vm.Errorf("bad argument #%d to '%s' (number or string expected, got %s)", idx, fname, vm.TypeName(v)))
	}
}

// numKeyValue returns integral float keys as Lua integers so a tree of
// inserted ints hands back ints from min/max.
func numKeyValue(f float64) vm.Value {
	if f == math.Trunc(f) && f >= math.MinInt64 && f < math.MaxInt64 {
		return int64(f)
	}
	return f
}

func (b *btreeBox) insert(fname string, idx int, v vm.Value) {
	f, s, isNum := btreeKey(fname, idx, v)
	if isNum {
		if b.strs != nil {
			panic(vm.Errorf("bad argument #%d to '%s' (btree holds string keys, got number)", idx, fname))
		}
		if b.nums == nil {
			b.nums = &BTree[float64]{maxKeys: b.maxKeys}
		}
		b.nums.Insert(f)
	} else {
		if b.nums != nil {
			panic(vm.Errorf("bad argument #%d to '%s' (btree holds number keys, got string)", idx, fname))
		}
		if b.strs == nil {
			b.strs = &BTree[string]{maxKeys: b.maxKeys}
		}
		b.strs.Insert(s)
	}
	b.count++
}

// search reports whether the key is present. A key whose type doesn't
// match the tree's key type is simply absent, not an error.
func (b *btreeBox) search(fname string, idx int, v vm.Value) bool {
	f, s, isNum := btreeKey(fname, idx, v)
	if isNum {
		return b.nums != nil && b.nums.Search(f)
	}
	return b.strs != nil && b.strs.Search(s)
}

// remove deletes one occurrence of the key, reporting whether it was found.
func (b *btreeBox) remove(fname string, idx int, v vm.Value) bool {
	if !b.search(fname, idx, v) {
		return false
	}
	f, s, isNum := btreeKey(fname, idx, v)
	if isNum {
		b.nums.Delete(f)
	} else {
		b.strs.Delete(s)
	}
	b.count--
	return true
}

// wrapBTree exposes a btreeBox as a Lua object: insert takes one or more
// keys; remove returns whether the key was found; min/max return nil on
// an empty tree.
func wrapBTree(b *btreeBox) *vm.Table {
	methods := vm.NewTable(0, 7)
	methods.Set("insert", &vm.GoFunc{Name: "btree:insert", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		for i := 2; i <= len(args); i++ {
			b.insert("btree:insert", i, args[i-1])
		}
		return nil
	}})
	methods.Set("search", &vm.GoFunc{Name: "btree:search", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{b.search("btree:search", 2, vm.AnyArg("btree:search", 2, args))}
	}})
	methods.Set("remove", &vm.GoFunc{Name: "btree:remove", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{b.remove("btree:remove", 2, vm.AnyArg("btree:remove", 2, args))}
	}})
	methods.Set("min", &vm.GoFunc{Name: "btree:min", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if b.nums != nil && b.nums.root != nil {
			return []vm.Value{numKeyValue(b.nums.root.Min())}
		}
		if b.strs != nil && b.strs.root != nil {
			return []vm.Value{b.strs.root.Min()}
		}
		return []vm.Value{nil}
	}})
	methods.Set("max", &vm.GoFunc{Name: "btree:max", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if b.nums != nil && b.nums.root != nil {
			return []vm.Value{numKeyValue(b.nums.root.Max())}
		}
		if b.strs != nil && b.strs.root != nil {
			return []vm.Value{b.strs.root.Max()}
		}
		return []vm.Value{nil}
	}})
	methods.Set("size", &vm.GoFunc{Name: "btree:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(b.count)}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "btree:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{b.count == 0}
	}})
	return withMethods(methods)
}

// withMethods builds a method-dispatch table over methods, attaches it
// via __index to a fresh table, and returns that table. All the wrap*
// helpers below follow the same shape.
func withMethods(methods *vm.Table) *vm.Table {
	t := vm.NewTable(0, 0)
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	t.SetMetatable(mt)
	return t
}

func wrapStack(s *Stack) *vm.Table {
	methods := vm.NewTable(0, 6)
	methods.Set("push", &vm.GoFunc{Name: "stack:push", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		s.Push(vm.AnyArg("stack:push", 2, args))
		return nil
	}})
	methods.Set("pop", &vm.GoFunc{Name: "stack:pop", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := s.Pop()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("peek", &vm.GoFunc{Name: "stack:peek", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := s.Peek()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("size", &vm.GoFunc{Name: "stack:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(s.Size())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "stack:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{s.Empty()}
	}})
	methods.Set("clear", &vm.GoFunc{Name: "stack:clear", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		s.Clear()
		return nil
	}})
	return withMethods(methods)
}

func wrapQueue(q *Queue) *vm.Table {
	methods := vm.NewTable(0, 6)
	methods.Set("push", &vm.GoFunc{Name: "queue:push", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		q.Push(vm.AnyArg("queue:push", 2, args))
		return nil
	}})
	methods.Set("pop", &vm.GoFunc{Name: "queue:pop", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := q.Pop()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("peek", &vm.GoFunc{Name: "queue:peek", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := q.Peek()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("size", &vm.GoFunc{Name: "queue:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(q.Size())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "queue:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{q.Empty()}
	}})
	methods.Set("clear", &vm.GoFunc{Name: "queue:clear", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		q.Clear()
		return nil
	}})
	return withMethods(methods)
}

func wrapDeque(d *Deque) *vm.Table {
	methods := vm.NewTable(0, 9)
	methods.Set("push_front", &vm.GoFunc{Name: "deque:push_front", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		d.PushFront(vm.AnyArg("deque:push_front", 2, args))
		return nil
	}})
	methods.Set("push_back", &vm.GoFunc{Name: "deque:push_back", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		d.PushBack(vm.AnyArg("deque:push_back", 2, args))
		return nil
	}})
	methods.Set("pop_front", &vm.GoFunc{Name: "deque:pop_front", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := d.PopFront()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("pop_back", &vm.GoFunc{Name: "deque:pop_back", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := d.PopBack()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("front", &vm.GoFunc{Name: "deque:front", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := d.Front()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("back", &vm.GoFunc{Name: "deque:back", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := d.Back()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("size", &vm.GoFunc{Name: "deque:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(d.Size())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "deque:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{d.Empty()}
	}})
	methods.Set("clear", &vm.GoFunc{Name: "deque:clear", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		d.Clear()
		return nil
	}})
	return withMethods(methods)
}

func wrapSet(s *Set) *vm.Table {
	methods := vm.NewTable(0, 7)
	methods.Set("add", &vm.GoFunc{Name: "set:add", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{s.Add(vm.AnyArg("set:add", 2, args))}
	}})
	methods.Set("remove", &vm.GoFunc{Name: "set:remove", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{s.Remove(vm.AnyArg("set:remove", 2, args))}
	}})
	methods.Set("contains", &vm.GoFunc{Name: "set:contains", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{s.Contains(vm.AnyArg("set:contains", 2, args))}
	}})
	methods.Set("size", &vm.GoFunc{Name: "set:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(s.Size())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "set:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{s.Empty()}
	}})
	methods.Set("values", &vm.GoFunc{Name: "set:values", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{valuesToTable(s.Values())}
	}})
	methods.Set("clear", &vm.GoFunc{Name: "set:clear", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		s.Clear()
		return nil
	}})
	return withMethods(methods)
}

func wrapList(l *LinkedList) *vm.Table {
	methods := vm.NewTable(0, 10)
	methods.Set("push_front", &vm.GoFunc{Name: "list:push_front", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		l.PushFront(vm.AnyArg("list:push_front", 2, args))
		return nil
	}})
	methods.Set("push_back", &vm.GoFunc{Name: "list:push_back", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		l.PushBack(vm.AnyArg("list:push_back", 2, args))
		return nil
	}})
	methods.Set("pop_front", &vm.GoFunc{Name: "list:pop_front", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := l.PopFront()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("pop_back", &vm.GoFunc{Name: "list:pop_back", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := l.PopBack()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("front", &vm.GoFunc{Name: "list:front", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := l.Front()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("back", &vm.GoFunc{Name: "list:back", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		v, ok := l.Back()
		if !ok {
			return []vm.Value{nil}
		}
		return []vm.Value{v}
	}})
	methods.Set("size", &vm.GoFunc{Name: "list:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(l.Size())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "list:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{l.Empty()}
	}})
	methods.Set("to_array", &vm.GoFunc{Name: "list:to_array", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{valuesToTable(l.ToArray())}
	}})
	methods.Set("clear", &vm.GoFunc{Name: "list:clear", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		l.Clear()
		return nil
	}})
	return withMethods(methods)
}

func wrapHeap(h *Heap[any]) *vm.Table {
	methods := vm.NewTable(0, 5)
	methods.Set("push", &vm.GoFunc{Name: "heap:push", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		h.Push(vm.AnyArg("heap:push", 2, args))
		return nil
	}})
	methods.Set("pop", &vm.GoFunc{Name: "heap:pop", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if h.Empty() {
			return []vm.Value{nil}
		}
		top := h.Top()
		h.Pop()
		return []vm.Value{top}
	}})
	methods.Set("top", &vm.GoFunc{Name: "heap:top", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if h.Empty() {
			return []vm.Value{nil}
		}
		return []vm.Value{h.Top()}
	}})
	methods.Set("size", &vm.GoFunc{Name: "heap:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(h.Size())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "heap:empty", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{h.Empty()}
	}})
	return withMethods(methods)
}

func wrapHashMap(hm *HashMap) *vm.Table {
	methods := vm.NewTable(0, 4)
	methods.Set("put", &vm.GoFunc{Name: "hashmap:put", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		k := vm.AnyArg("hashmap:put", 2, args)
		v := vm.AnyArg("hashmap:put", 3, args)
		hm.Put(k, v)
		return nil
	}})
	methods.Set("get", &vm.GoFunc{Name: "hashmap:get", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		k := vm.AnyArg("hashmap:get", 2, args)
		return []vm.Value{hm.Get(k)}
	}})
	methods.Set("contains", &vm.GoFunc{Name: "hashmap:contains", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		k := vm.AnyArg("hashmap:contains", 2, args)
		return []vm.Value{hm.Contains(k)}
	}})
	methods.Set("size", &vm.GoFunc{Name: "hashmap:size", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(hm.size)}
	}})
	return withMethods(methods)
}

// valuesToTable copies a Go slice into a fresh 1-indexed Lua table.
// Shared by Set:values and LinkedList:to_array.
func valuesToTable(vs []any) *vm.Table {
	t := vm.NewTable(len(vs), 0)
	for i, v := range vs {
		t.Set(int64(i+1), v)
	}
	return t
}
