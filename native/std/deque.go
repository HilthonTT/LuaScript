package std

// Deque is a double-ended queue over arbitrary Lua values. Backed by a
// ring buffer so PushFront/PushBack/PopFront/PopBack are all O(1)
// amortized. The buffer doubles when full and is never shrunk —
// real-world deques rarely benefit from shrinkage and the bookkeeping
// makes the iteration paths uglier.
type Deque struct {
	buf  []any
	head int // index of the front element when size > 0
	size int
}

// NewDeque returns an empty deque.
func NewDeque() *Deque {
	return &Deque{}
}

// Size returns the number of elements.
func (d *Deque) Size() int {
	return d.size
}

// Empty reports whether the deque is empty.
func (d *Deque) Empty() bool {
	return d.size == 0
}

// PushBack appends v to the tail.
func (d *Deque) PushBack(v any) {
	d.growIfFull()
	tail := (d.head + d.size) & (len(d.buf) - 1)
	d.buf[tail] = v
	d.size++
}

// PushFront prepends v to the head.
func (d *Deque) PushFront(v any) {
	d.growIfFull()
	d.head = (d.head - 1) & (len(d.buf) - 1)
	d.buf[d.head] = v
	d.size++
}

// PopFront removes and returns the head element.
func (d *Deque) PopFront() (any, bool) {
	if d.size == 0 {
		return nil, false
	}
	v := d.buf[d.head]
	d.buf[d.head] = nil
	d.head = (d.head + 1) & (len(d.buf) - 1)
	d.size--
	return v, true
}

// PopBack removes and returns the tail element.
func (d *Deque) PopBack() (any, bool) {
	if d.size == 0 {
		return nil, false
	}
	tail := (d.head + d.size - 1) & (len(d.buf) - 1)
	v := d.buf[tail]
	d.buf[tail] = nil
	d.size--
	return v, true
}

// Front returns the head element without removing it.
func (d *Deque) Front() (any, bool) {
	if d.size == 0 {
		return nil, false
	}
	return d.buf[d.head], true
}

// Back returns the tail element without removing it.
func (d *Deque) Back() (any, bool) {
	if d.size == 0 {
		return nil, false
	}
	tail := (d.head + d.size - 1) & (len(d.buf) - 1)
	return d.buf[tail], true
}

// Clear drops every element. Capacity is preserved.
func (d *Deque) Clear() {
	for i := range d.buf {
		d.buf[i] = nil
	}
	d.head = 0
	d.size = 0
}

// growIfFull doubles the ring buffer when it's about to overflow.
// Buffer length is always a power of two so the index math can use
// `& (len-1)` instead of `%`.
func (d *Deque) growIfFull() {
	if len(d.buf) == 0 {
		d.buf = make([]any, 8)
		return
	}
	if d.size < len(d.buf) {
		return
	}
	nbuf := make([]any, len(d.buf)*2)
	// Unwrap into the new buffer starting at index 0.
	n := copy(nbuf, d.buf[d.head:])
	copy(nbuf[n:], d.buf[:d.head])
	d.buf = nbuf
	d.head = 0
}
