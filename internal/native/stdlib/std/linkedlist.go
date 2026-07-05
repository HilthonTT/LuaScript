package std

// listNode is the internal node type for LinkedList. Doubly linked so
// every operation at either end is O(1).
type listNode struct {
	value      any
	prev, next *listNode
}

// LinkedList is a doubly-linked list of Lua values. All end-operations
// are O(1); ToArray is O(n). For random-access use a Deque or a plain
// Lua table — this type exists for cases where you actually want
// stable interior references / iteration.
type LinkedList struct {
	head, tail *listNode
	size       int
}

// NewLinkedList returns an empty list.
func NewLinkedList() *LinkedList {
	return &LinkedList{}
}

// Size returns the number of elements.
func (l *LinkedList) Size() int {
	return l.size
}

// Empty reports whether the list has no elements.
func (l *LinkedList) Empty() bool {
	return l.size == 0
}

// PushFront prepends v to the head.
func (l *LinkedList) PushFront(v any) {
	n := &listNode{value: v, next: l.head}
	if l.head != nil {
		l.head.prev = n
	} else {
		l.tail = n
	}
	l.head = n
	l.size++
}

// PushBack appends v to the tail.
func (l *LinkedList) PushBack(v any) {
	n := &listNode{value: v, prev: l.tail}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.size++
}

// PopFront removes and returns the head element.
func (l *LinkedList) PopFront() (any, bool) {
	if l.head == nil {
		return nil, false
	}
	n := l.head
	l.head = n.next
	if l.head != nil {
		l.head.prev = nil
	} else {
		l.tail = nil
	}
	l.size--
	return n.value, true
}

// PopBack removes and returns the tail element.
func (l *LinkedList) PopBack() (any, bool) {
	if l.tail == nil {
		return nil, false
	}
	n := l.tail
	l.tail = n.prev
	if l.tail != nil {
		l.tail.next = nil
	} else {
		l.head = nil
	}
	l.size--
	return n.value, true
}

// Front returns the head element without removing it.
func (l *LinkedList) Front() (any, bool) {
	if l.head == nil {
		return nil, false
	}
	return l.head.value, true
}

// Back returns the tail element without removing it.
func (l *LinkedList) Back() (any, bool) {
	if l.tail == nil {
		return nil, false
	}
	return l.tail.value, true
}

// ToArray walks the list head→tail and returns a snapshot slice.
func (l *LinkedList) ToArray() []any {
	out := make([]any, 0, l.size)
	for n := l.head; n != nil; n = n.next {
		out = append(out, n.value)
	}
	return out
}

// Clear drops every element.
func (l *LinkedList) Clear() {
	l.head = nil
	l.tail = nil
	l.size = 0
}
