package std

type listNode struct {
	value      any
	prev, next *listNode
}

type LinkedList struct {
	head, tail *listNode
	size       int
}

func NewLinkedList() *LinkedList {
	return &LinkedList{}
}

func (l *LinkedList) Size() int {
	return l.size
}

func (l *LinkedList) Empty() bool {
	return l.size == 0
}

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

func (l *LinkedList) Front() (any, bool) {
	if l.head == nil {
		return nil, false
	}
	return l.head.value, true
}

func (l *LinkedList) Back() (any, bool) {
	if l.tail == nil {
		return nil, false
	}
	return l.tail.value, true
}

func (l *LinkedList) ToArray() []any {
	out := make([]any, 0, l.size)
	for n := l.head; n != nil; n = n.next {
		out = append(out, n.value)
	}
	return out
}

func (l *LinkedList) Clear() {
	l.head = nil
	l.tail = nil
	l.size = 0
}
