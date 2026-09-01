package std

type Queue struct {
	items []any
	head  int
}

func NewQueue() *Queue {
	return &Queue{}
}

func (q *Queue) Push(v any) {
	q.items = append(q.items, v)
}

func (q *Queue) Pop() (any, bool) {
	if q.head >= len(q.items) {
		return nil, false
	}
	v := q.items[q.head]
	q.items[q.head] = nil
	q.head++
	if q.head > 16 && q.head*2 >= len(q.items) {
		n := copy(q.items, q.items[q.head:])
		for i := n; i < len(q.items); i++ {
			q.items[i] = nil
		}
		q.items = q.items[:n]
		q.head = 0
	}
	return v, true
}

func (q *Queue) Peek() (any, bool) {
	if q.head >= len(q.items) {
		return nil, false
	}
	return q.items[q.head], true
}

func (q *Queue) Size() int {
	return len(q.items) - q.head
}

func (q *Queue) Empty() bool {
	return q.head >= len(q.items)
}

func (q *Queue) Clear() {
	for i := range q.items {
		q.items[i] = nil
	}
	q.items = q.items[:0]
	q.head = 0
}
