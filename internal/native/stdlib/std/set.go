package std

type Set struct {
	m map[any]struct{}
}

func NewSet() *Set {
	return &Set{m: make(map[any]struct{})}
}

func (s *Set) Add(v any) bool {
	if _, ok := s.m[v]; ok {
		return false
	}
	s.m[v] = struct{}{}
	return true
}

func (s *Set) Remove(v any) bool {
	if _, ok := s.m[v]; !ok {
		return false
	}
	delete(s.m, v)
	return true
}

func (s *Set) Contains(v any) bool {
	_, ok := s.m[v]
	return ok
}

func (s *Set) Size() int {
	return len(s.m)
}

func (s *Set) Empty() bool {
	return len(s.m) == 0
}

func (s *Set) Values() []any {
	out := make([]any, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	return out
}

func (s *Set) Clear() {
	for k := range s.m {
		delete(s.m, k)
	}
}
