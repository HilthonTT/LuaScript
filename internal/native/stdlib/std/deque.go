package std

type Deque struct {
	buf  []any
	head int
	size int
}

func NewDeque() *Deque {
	return &Deque{}
}

func (d *Deque) Size() int {
	return d.size
}

func (d *Deque) Empty() bool {
	return d.size == 0
}

func (d *Deque) PushBack(v any) {
	d.growIfFull()
	tail := (d.head + d.size) & (len(d.buf) - 1)
	d.buf[tail] = v
	d.size++
}

func (d *Deque) PushFront(v any) {
	d.growIfFull()
	d.head = (d.head - 1) & (len(d.buf) - 1)
	d.buf[d.head] = v
	d.size++
}

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

func (d *Deque) Front() (any, bool) {
	if d.size == 0 {
		return nil, false
	}
	return d.buf[d.head], true
}

func (d *Deque) Back() (any, bool) {
	if d.size == 0 {
		return nil, false
	}
	tail := (d.head + d.size - 1) & (len(d.buf) - 1)
	return d.buf[tail], true
}

func (d *Deque) Clear() {
	for i := range d.buf {
		d.buf[i] = nil
	}
	d.head = 0
	d.size = 0
}

func (d *Deque) growIfFull() {
	if len(d.buf) == 0 {
		d.buf = make([]any, 8)
		return
	}
	if d.size < len(d.buf) {
		return
	}
	nbuf := make([]any, len(d.buf)*2)
	n := copy(nbuf, d.buf[d.head:])
	copy(nbuf[n:], d.buf[:d.head])
	d.buf = nbuf
	d.head = 0
}
