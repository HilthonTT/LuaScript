package vm

import (
	"strings"
)

// Lua pattern matching — a faithful port of PUC Lua's lstrlib.c pattern
// engine, expressed in idiomatic Go. The pattern language is documented
// in Lua 5.4 §6.4.1; it is NOT PCRE/regex — quantifiers are single-char
// only, there is no alternation, captures use parens, anchors are `^`
// and `$`, character classes use `%a %d %s ...` etc.
//
// Entry points:
//   - PatternFind:  string.find(s, pat, init, plain=false)
//   - PatternMatch: string.match(s, pat, init)
//   - PatternGMatch: iterator for string.gmatch(s, pat)
//   - PatternGSub:  string.gsub(s, pat, repl, maxN)
//
// All four sit on top of the same matchState driver. A single
// matchState owns the source string, the current pattern, the capture
// stack, and a recursion budget guarding catastrophic backtracking on
// pathological patterns.

const (
	patternMaxCaptures = 32
	patternMaxDepth    = 200
	// capPosition is the sentinel `len` value for a position capture
	// (the empty `()` syntax) — Lua's lstrlib uses -1 / -2 as flags.
	capPosition = -1
	capUnfinished = -2
)

type patternCapture struct {
	start int // byte offset in s
	len   int // byte length, or capPosition / capUnfinished
}

type matchState struct {
	src      string
	pat      string
	captures []patternCapture
	depth    int
}

// PatternHasSpecials reports whether pat contains any pattern-magic
// characters. string.find's `plain` flag short-circuits when this is
// false, falling back to strings.Index without engaging the matcher.
func PatternHasSpecials(pat string) bool {
	const magic = "^$*+?.([%-"
	return strings.ContainsAny(pat, magic)
}

// PatternFind searches s for pat starting from init (1-based, like Lua;
// 0/negative values clamp). Returns (startByte, endByte, captures, ok).
// startByte / endByte are 1-based byte offsets, matching Lua's
// string.find return shape.
func PatternFind(s, pat string, init int) (int, int, []Value, bool) {
	ms := matchState{src: s, pat: pat, captures: make([]patternCapture, 0, 4)}
	start := luaInit(s, init)
	anchored := strings.HasPrefix(pat, "^")
	pStart := 0
	if anchored {
		pStart = 1
	}
	for sStart := start; sStart <= len(s); sStart++ {
		ms.captures = ms.captures[:0]
		end := ms.match(sStart, pStart)
		if end >= 0 {
			return sStart + 1, end, ms.collectCaptures(sStart, end), true
		}
		if anchored {
			break
		}
	}
	return 0, 0, nil, false
}

// PatternMatch is like PatternFind but only returns the captures (or
// the whole match if there are no explicit captures), matching
// string.match's contract.
func PatternMatch(s, pat string, init int) []Value {
	sStart, end, caps, ok := PatternFind(s, pat, init)
	if !ok {
		return []Value{nil}
	}
	if len(caps) > 0 {
		return caps
	}
	return []Value{s[sStart-1 : end]}
}

// PatternGMatchNext drives one step of a gmatch iterator. `state` is the
// (string, pattern, position) iterator state stored as a Lua table; the
// caller advances `pos` between calls.
type GMatchIter struct {
	s, pat string
	pos    int // 1-based byte position to start the next search from
}

// NewGMatchIter sets up a string.gmatch iterator state.
func NewGMatchIter(s, pat string) *GMatchIter {
	return &GMatchIter{s: s, pat: pat, pos: 1}
}

// Next produces the next match's captures (or whole-match), or nil when
// exhausted. The iterator advances past zero-length matches to avoid
// infinite loops on patterns like "a*".
func (g *GMatchIter) Next() []Value {
	if g.pos > len(g.s)+1 {
		return nil
	}
	startByte, endByte, caps, ok := PatternFind(g.s, g.pat, g.pos)
	if !ok {
		return nil
	}
	// Advance past this match; if it was zero-length, step one byte to
	// avoid an infinite loop.
	if endByte+1 > startByte {
		g.pos = endByte + 1
	} else {
		g.pos = startByte + 1
	}
	if len(caps) > 0 {
		return caps
	}
	return []Value{g.s[startByte-1 : endByte]}
}

// PatternGSub implements string.gsub. repl is the substitution source
// (string, *Table, or *GoFunc/*Closure); the function form is called
// back through callFn (the caller injects vm.CallValue to keep this
// file VM-API-free). maxN <= 0 means "unlimited".
func PatternGSub(
	s, pat string, repl Value, maxN int,
	callFn func(fn Value, args []Value) []Value,
) (string, int) {
	if maxN <= 0 {
		maxN = 1<<31 - 1
	}
	var b strings.Builder
	b.Grow(len(s))
	anchored := strings.HasPrefix(pat, "^")
	pStart := 0
	if anchored {
		pStart = 1
	}
	count := 0
	srcPos := 0
	for srcPos <= len(s) && count < maxN {
		ms := matchState{src: s, pat: pat, captures: make([]patternCapture, 0, 4)}
		end := ms.match(srcPos, pStart)
		if end < 0 {
			if anchored {
				break
			}
			if srcPos < len(s) {
				b.WriteByte(s[srcPos])
			}
			srcPos++
			continue
		}
		caps := ms.collectCaptures(srcPos, end)
		matchStr := s[srcPos:end]
		applyReplacement(&b, matchStr, caps, repl, callFn)
		count++
		// Advance past the match; zero-length match: emit one source char
		// to make progress.
		if end > srcPos {
			srcPos = end
		} else {
			if srcPos < len(s) {
				b.WriteByte(s[srcPos])
			}
			srcPos++
		}
		if anchored {
			break
		}
	}
	if srcPos < len(s) {
		b.WriteString(s[srcPos:])
	}
	return b.String(), count
}

// applyReplacement writes the substitution for one match into b. Mirrors
// Lua's three replacement modes.
func applyReplacement(
	b *strings.Builder, matchStr string, caps []Value,
	repl Value, callFn func(fn Value, args []Value) []Value,
) {
	switch r := repl.(type) {
	case string:
		// "%0" is the whole match; "%1".."%9" are explicit captures;
		// "%%" is a literal %.
		for i := 0; i < len(r); i++ {
			c := r[i]
			if c != '%' || i+1 >= len(r) {
				b.WriteByte(c)
				continue
			}
			i++
			d := r[i]
			switch {
			case d == '%':
				b.WriteByte('%')
			case d == '0':
				b.WriteString(matchStr)
			case d >= '1' && d <= '9':
				idx := int(d - '1')
				if idx < len(caps) {
					b.WriteString(ToString(caps[idx]))
				}
			default:
				b.WriteByte('%')
				b.WriteByte(d)
			}
		}
	case *Table:
		// Key is the first capture, or the whole match if no captures.
		var key Value
		if len(caps) > 0 {
			key = caps[0]
		} else {
			key = matchStr
		}
		v := r.Get(key)
		if v == nil || v == false {
			b.WriteString(matchStr)
		} else if s, ok := v.(string); ok {
			b.WriteString(s)
		} else {
			b.WriteString(ToString(v))
		}
	default:
		// Function (Closure or GoFunc): call with captures (or whole match).
		args := caps
		if len(args) == 0 {
			args = []Value{matchStr}
		}
		results := callFn(repl, args)
		var v Value
		if len(results) > 0 {
			v = results[0]
		}
		if v == nil || v == false {
			b.WriteString(matchStr)
		} else if s, ok := v.(string); ok {
			b.WriteString(s)
		} else {
			b.WriteString(ToString(v))
		}
	}
}

// luaInit converts Lua's 1-based init (with negative-from-end support)
// into a 0-based byte offset that's clamped to [0, len(s)].
func luaInit(s string, init int) int {
	if init < 0 {
		init = len(s) + 1 + init
	}
	if init < 1 {
		init = 1
	}
	return init - 1
}

// collectCaptures resolves the matchState's capture stack into a slice
// of Values (strings for normal captures, int64 for position captures).
// `sStart..matchEnd` is the overall match boundary; needed for the
// implicit "whole match" return when there were no explicit captures.
func (ms *matchState) collectCaptures(sStart, matchEnd int) []Value {
	_ = matchEnd
	if len(ms.captures) == 0 {
		return nil
	}
	out := make([]Value, len(ms.captures))
	for i, c := range ms.captures {
		switch c.len {
		case capPosition:
			out[i] = int64(c.start + 1) // 1-based
		case capUnfinished:
			// Open capture — pattern is malformed but produce something.
			out[i] = ""
		default:
			out[i] = ms.src[c.start : c.start+c.len]
		}
	}
	_ = sStart
	return out
}

// ---------------------------------------------------------------------------
// Core matcher
// ---------------------------------------------------------------------------

// match is the central recursive routine. It returns the byte offset
// just past the end of the match, or -1 on failure. Both arguments are
// indices: sIdx into ms.src, pIdx into ms.pat.
func (ms *matchState) match(sIdx, pIdx int) int {
	ms.depth++
	if ms.depth > patternMaxDepth {
		ms.depth--
		return -1
	}
	defer func() { ms.depth-- }()

	for {
		if pIdx >= len(ms.pat) {
			return sIdx
		}
		p := ms.pat[pIdx]

		switch p {
		case '(':
			// Open capture — `()` (position) or `(...)` (substring).
			if pIdx+1 < len(ms.pat) && ms.pat[pIdx+1] == ')' {
				return ms.startCapture(sIdx, pIdx+2, capPosition)
			}
			return ms.startCapture(sIdx, pIdx+1, capUnfinished)
		case ')':
			return ms.closeCapture(sIdx, pIdx+1)
		case '$':
			// End-anchor when last char of pattern; otherwise literal.
			if pIdx+1 == len(ms.pat) {
				if sIdx == len(ms.src) {
					return sIdx
				}
				return -1
			}
			// fall through to literal handling
		case '%':
			if pIdx+1 < len(ms.pat) {
				next := ms.pat[pIdx+1]
				if next >= '1' && next <= '9' {
					return ms.matchBackref(sIdx, pIdx+2, int(next-'1'))
				}
				if next == 'b' {
					return ms.matchBalance(sIdx, pIdx+2)
				}
				if next == 'f' {
					return ms.matchFrontier(sIdx, pIdx+2)
				}
			}
		}

		// Single-char pattern unit + optional quantifier.
		patEnd := ms.singleClassEnd(pIdx)
		if patEnd < 0 {
			return -1
		}
		quant := byte(0)
		if patEnd < len(ms.pat) {
			q := ms.pat[patEnd]
			if q == '?' || q == '*' || q == '+' || q == '-' {
				quant = q
			}
		}

		switch quant {
		case 0:
			if sIdx >= len(ms.src) || !ms.singleMatch(ms.src[sIdx], pIdx) {
				return -1
			}
			sIdx++
			pIdx = patEnd
		case '?':
			if sIdx < len(ms.src) && ms.singleMatch(ms.src[sIdx], pIdx) {
				if r := ms.match(sIdx+1, patEnd+1); r >= 0 {
					return r
				}
			}
			pIdx = patEnd + 1
		case '+':
			if sIdx >= len(ms.src) || !ms.singleMatch(ms.src[sIdx], pIdx) {
				return -1
			}
			return ms.matchMax(sIdx+1, pIdx, patEnd+1)
		case '*':
			return ms.matchMax(sIdx, pIdx, patEnd+1)
		case '-':
			return ms.matchMin(sIdx, pIdx, patEnd+1)
		}
	}
}

// singleClassEnd returns the index just past one single-char class
// starting at pIdx. Classes: `%X`, `[...]`, or a single literal char.
// Returns -1 on a malformed pattern.
func (ms *matchState) singleClassEnd(pIdx int) int {
	if pIdx >= len(ms.pat) {
		return -1
	}
	switch ms.pat[pIdx] {
	case '%':
		if pIdx+1 >= len(ms.pat) {
			return -1
		}
		return pIdx + 2
	case '[':
		end := pIdx + 1
		if end < len(ms.pat) && ms.pat[end] == '^' {
			end++
		}
		// First char inside [] may itself be ']' as a literal.
		if end < len(ms.pat) && ms.pat[end] == ']' {
			end++
		}
		for end < len(ms.pat) && ms.pat[end] != ']' {
			if ms.pat[end] == '%' && end+1 < len(ms.pat) {
				end++ // skip escape
			}
			end++
		}
		if end >= len(ms.pat) {
			return -1
		}
		return end + 1
	default:
		return pIdx + 1
	}
}

// singleMatch tests whether a single source byte matches the class at
// ms.pat[pIdx..singleClassEnd(pIdx)].
func (ms *matchState) singleMatch(c byte, pIdx int) bool {
	switch ms.pat[pIdx] {
	case '.':
		return true
	case '%':
		return matchClass(c, ms.pat[pIdx+1])
	case '[':
		return ms.matchBracket(c, pIdx)
	default:
		return c == ms.pat[pIdx]
	}
}

// matchClass tests one character against a Lua class letter (`a` for
// letter, etc.). Uppercase letters complement.
func matchClass(c, cls byte) bool {
	var res bool
	lower := cls | 0x20
	switch lower {
	case 'a':
		res = (c|0x20) >= 'a' && (c|0x20) <= 'z'
	case 'c':
		res = c < 0x20 || c == 0x7F
	case 'd':
		res = c >= '0' && c <= '9'
	case 'g':
		res = c > 0x20 && c < 0x7F
	case 'l':
		res = c >= 'a' && c <= 'z'
	case 'p':
		res = isPunct(c)
	case 's':
		res = c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
	case 'u':
		res = c >= 'A' && c <= 'Z'
	case 'w':
		res = (c >= '0' && c <= '9') || ((c|0x20) >= 'a' && (c|0x20) <= 'z')
	case 'x':
		res = (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	default:
		// Not a class letter — `%X` matches literal X.
		return c == cls
	}
	if cls >= 'A' && cls <= 'Z' {
		return !res
	}
	return res
}

func isPunct(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') ||
		(c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

// matchBracket evaluates one source byte against a `[...]` class.
func (ms *matchState) matchBracket(c byte, pIdx int) bool {
	idx := pIdx + 1
	negate := false
	if idx < len(ms.pat) && ms.pat[idx] == '^' {
		negate = true
		idx++
	}
	found := false
	for idx < len(ms.pat) && ms.pat[idx] != ']' {
		if ms.pat[idx] == '%' && idx+1 < len(ms.pat) {
			if matchClass(c, ms.pat[idx+1]) {
				found = true
			}
			idx += 2
			continue
		}
		// Range? `a-z` shape.
		if idx+2 < len(ms.pat) && ms.pat[idx+1] == '-' && ms.pat[idx+2] != ']' {
			if c >= ms.pat[idx] && c <= ms.pat[idx+2] {
				found = true
			}
			idx += 3
			continue
		}
		if ms.pat[idx] == c {
			found = true
		}
		idx++
	}
	if negate {
		return !found
	}
	return found
}

// matchMax handles `+`/`*`: consume greedily then back off.
func (ms *matchState) matchMax(sIdx, pIdx, pAfter int) int {
	count := 0
	for sIdx+count < len(ms.src) && ms.singleMatch(ms.src[sIdx+count], pIdx) {
		count++
	}
	for count >= 0 {
		if r := ms.match(sIdx+count, pAfter); r >= 0 {
			return r
		}
		count--
	}
	return -1
}

// matchMin handles `-`: consume lazily, growing only as needed.
func (ms *matchState) matchMin(sIdx, pIdx, pAfter int) int {
	for {
		if r := ms.match(sIdx, pAfter); r >= 0 {
			return r
		}
		if sIdx < len(ms.src) && ms.singleMatch(ms.src[sIdx], pIdx) {
			sIdx++
		} else {
			return -1
		}
	}
}

// startCapture begins a capture and recurses; on failure, rolls back.
func (ms *matchState) startCapture(sIdx, pIdx, capLen int) int {
	if len(ms.captures) >= patternMaxCaptures {
		return -1
	}
	ms.captures = append(ms.captures, patternCapture{start: sIdx, len: capLen})
	r := ms.match(sIdx, pIdx)
	if r < 0 {
		ms.captures = ms.captures[:len(ms.captures)-1]
	}
	return r
}

// closeCapture closes the most recent unfinished capture.
func (ms *matchState) closeCapture(sIdx, pIdx int) int {
	idx := ms.findUnfinishedCapture()
	if idx < 0 {
		return -1
	}
	ms.captures[idx].len = sIdx - ms.captures[idx].start
	r := ms.match(sIdx, pIdx)
	if r < 0 {
		ms.captures[idx].len = capUnfinished
	}
	return r
}

func (ms *matchState) findUnfinishedCapture() int {
	for i := len(ms.captures) - 1; i >= 0; i-- {
		if ms.captures[i].len == capUnfinished {
			return i
		}
	}
	return -1
}

// matchBackref tests s at sIdx against the contents of capture[n].
func (ms *matchState) matchBackref(sIdx, pIdx, n int) int {
	if n < 0 || n >= len(ms.captures) {
		return -1
	}
	c := ms.captures[n]
	if c.len < 0 {
		return -1
	}
	captured := ms.src[c.start : c.start+c.len]
	if sIdx+len(captured) > len(ms.src) {
		return -1
	}
	if ms.src[sIdx:sIdx+len(captured)] != captured {
		return -1
	}
	return ms.match(sIdx+len(captured), pIdx)
}

// matchBalance handles `%b<open><close>` — match a balanced pair.
func (ms *matchState) matchBalance(sIdx, pIdx int) int {
	if pIdx+1 >= len(ms.pat) {
		return -1
	}
	open, close := ms.pat[pIdx], ms.pat[pIdx+1]
	if sIdx >= len(ms.src) || ms.src[sIdx] != open {
		return -1
	}
	depth := 1
	i := sIdx + 1
	for i < len(ms.src) {
		switch ms.src[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return ms.match(i+1, pIdx+2)
			}
		}
		i++
	}
	return -1
}

// matchFrontier handles `%f[set]` — a zero-width assertion: the previous
// source char must NOT be in [set] and the current one must.
func (ms *matchState) matchFrontier(sIdx, pIdx int) int {
	if pIdx >= len(ms.pat) || ms.pat[pIdx] != '[' {
		return -1
	}
	classStart := pIdx
	classEnd := ms.singleClassEnd(classStart)
	if classEnd < 0 {
		return -1
	}
	var prev byte
	if sIdx > 0 {
		prev = ms.src[sIdx-1]
	}
	var curr byte
	if sIdx < len(ms.src) {
		curr = ms.src[sIdx]
	}
	if ms.matchBracket(prev, classStart) {
		return -1
	}
	if !ms.matchBracket(curr, classStart) {
		return -1
	}
	return ms.match(sIdx, classEnd)
}
