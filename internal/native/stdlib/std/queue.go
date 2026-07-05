package std

// Queue is a FIFO collection over arbitrary Lua values. Backed by a
// slice with a head index — Push is amortized O(1) and Pop is O(1).
// When the head wanders past half the underlying buffer the tail half
// is shifted down so we don't leak unbounded capacity.
type Queue struct {
	items []any
	head  int
}

// NewQueue returns an empty queue.
func NewQueue() *Queue {
	return &Queue{}
}

// Push appends v to the back of the queue.
func (q *Queue) Push(v any) {
	q.items = append(q.items, v)
}

// Pop removes and returns the front element. The second return is
// false if the queue was empty.
func (q *Queue) Pop() (any, bool) {
	if q.head >= len(q.items) {
		return nil, false
	}
	v := q.items[q.head]
	q.items[q.head] = nil
	q.head++
	if q.head > 16 && q.head*2 >= len(q.items) {
		// Shift the live window back to index 0 so the head pointer
		// doesn't grow without bound for long-lived queues.
		n := copy(q.items, q.items[q.head:])
		for i := n; i < len(q.items); i++ {
			q.items[i] = nil
		}
		q.items = q.items[:n]
		q.head = 0
	}
	return v, true
}

// Peek returns the front element without removing it.
func (q *Queue) Peek() (any, bool) {
	if q.head >= len(q.items) {
		return nil, false
	}
	return q.items[q.head], true
}

// Size returns the number of live elements.
func (q *Queue) Size() int {
	return len(q.items) - q.head
}

// Empty reports whether the queue has no elements.
func (q *Queue) Empty() bool {
	return q.head >= len(q.items)
}

// Clear drops every element. Capacity is preserved.
func (q *Queue) Clear() {
	for i := range q.items {
		q.items[i] = nil
	}
	q.items = q.items[:0]
	q.head = 0
}
