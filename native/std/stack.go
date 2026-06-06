package std

// Stack is a LIFO collection over arbitrary Lua values. Backed by a
// plain slice — Push/Pop/Peek are amortized O(1).
type Stack struct {
	items []any
}

// NewStack returns an empty stack.
func NewStack() *Stack {
	return &Stack{}
}

// Push appends v to the top of the stack.
func (s *Stack) Push(v any) {
	s.items = append(s.items, v)
}

// Pop removes and returns the top element. The second return is false
// if the stack was empty.
func (s *Stack) Pop() (any, bool) {
	n := len(s.items)
	if n == 0 {
		return nil, false
	}
	v := s.items[n-1]
	s.items[n-1] = nil
	s.items = s.items[:n-1]
	return v, true
}

// Peek returns the top element without removing it.
func (s *Stack) Peek() (any, bool) {
	n := len(s.items)
	if n == 0 {
		return nil, false
	}
	return s.items[n-1], true
}

// Size returns the number of elements.
func (s *Stack) Size() int {
	return len(s.items)
}

// Empty reports whether the stack has no elements.
func (s *Stack) Empty() bool {
	return len(s.items) == 0
}

// Clear drops every element. Capacity is preserved so subsequent
// pushes don't immediately re-allocate.
func (s *Stack) Clear() {
	for i := range s.items {
		s.items[i] = nil
	}
	s.items = s.items[:0]
}
