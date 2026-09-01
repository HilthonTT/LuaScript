package std

type Stack struct {
	items []any
}

func NewStack() *Stack {
	return &Stack{}
}

func (s *Stack) Push(v any) {
	s.items = append(s.items, v)
}

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

func (s *Stack) Peek() (any, bool) {
	n := len(s.items)
	if n == 0 {
		return nil, false
	}
	return s.items[n-1], true
}

func (s *Stack) Size() int {
	return len(s.items)
}

func (s *Stack) Empty() bool {
	return len(s.items) == 0
}

func (s *Stack) Clear() {
	for i := range s.items {
		s.items[i] = nil
	}
	s.items = s.items[:0]
}
