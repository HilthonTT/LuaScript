package std

// Set is an unordered collection of unique Lua values. Membership uses
// Go's native map, which means key equality follows Go's rules:
// strings, ints, floats, bools and nil all interoperate naturally, and
// reference values (tables, closures) compare by identity. Sets do NOT
// honour Lua's float-vs-int equality (1 == 1.0 in Lua but distinct in
// a Go map) — callers should pre-normalise if they care.
type Set struct {
	m map[any]struct{}
}

// NewSet returns an empty set.
func NewSet() *Set {
	return &Set{m: make(map[any]struct{})}
}

// Add inserts v. Returns true if v was newly added, false if already
// present — handy for "if added then ..." flows.
func (s *Set) Add(v any) bool {
	if _, ok := s.m[v]; ok {
		return false
	}
	s.m[v] = struct{}{}
	return true
}

// Remove deletes v. Returns true if v was present.
func (s *Set) Remove(v any) bool {
	if _, ok := s.m[v]; !ok {
		return false
	}
	delete(s.m, v)
	return true
}

// Contains reports whether v is in the set.
func (s *Set) Contains(v any) bool {
	_, ok := s.m[v]
	return ok
}

// Size returns the number of elements.
func (s *Set) Size() int {
	return len(s.m)
}

// Empty reports whether the set is empty.
func (s *Set) Empty() bool {
	return len(s.m) == 0
}

// Values returns a snapshot slice of every element. Order is the
// underlying map iteration order — i.e. random per call.
func (s *Set) Values() []any {
	out := make([]any, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	return out
}

// Clear drops every element.
func (s *Set) Clear() {
	for k := range s.m {
		delete(s.m, k)
	}
}
