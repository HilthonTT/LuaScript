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
	capPosition   = -1
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
	return ms.find(init)
}

// find runs the search loop, reusing ms's capture buffer across attempts
// (and across calls, for callers that keep the matchState alive).
func (ms *matchState) find(init int) (int, int, []Value, bool) {
	return ms.findImpl(init, true)
}

// findImpl is find with explicit control over `^` anchoring. gmatch passes
// allowAnchor=false: Lua 5.4 documents that a leading '^' does NOT anchor in
// gmatch (it would prevent iteration), so there it is matched as an ordinary
// character instead.
func (ms *matchState) findImpl(init int, allowAnchor bool) (int, int, []Value, bool) {
	start := luaInit(ms.src, init)
	anchored := allowAnchor && strings.HasPrefix(ms.pat, "^")
	pStart := 0
	if anchored {
		pStart = 1
	}
	for sStart := start; sStart <= len(ms.src); sStart++ {
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
	ms        matchState
	pos       int // 1-based byte position to start the next search from
	lastMatch int // byte end of the previous match, or -1 (Lua 5.4 rule)
}

// NewGMatchIter sets up a string.gmatch iterator state. `init` is the
// Lua 5.4 optional third argument: a 1-based start position (negative
// counts from the end, clamped to [1, #s+1]).
func NewGMatchIter(s, pat string, init int) *GMatchIter {
	return &GMatchIter{
		ms:        matchState{src: s, pat: pat, captures: make([]patternCapture, 0, 4)},
		pos:       luaInit(s, init) + 1,
		lastMatch: -1,
	}
}

// Next produces the next match's captures (or whole-match), or nil when
// exhausted. Per Lua 5.4 (`e != lastmatch` in gmatch_aux), an empty match
// abutting the previous match's end is rejected and the scan resumes one
// byte later — that both avoids infinite loops on patterns like "a*" and
// stops a spurious empty match right after a non-empty one. The matchState
// (and its capture buffer) is reused across calls, so a gmatch loop
// allocates per match only for the returned values.
func (g *GMatchIter) Next() []Value {
	for g.pos <= len(g.ms.src)+1 {
		startByte, endByte, caps, ok := g.ms.findImpl(g.pos, false)
		if !ok {
			return nil
		}
		if endByte == g.lastMatch && endByte == startByte-1 {
			// Empty match ending where the previous match ended: rejected.
			g.pos = startByte + 1
			continue
		}
		g.lastMatch = endByte
		// Next scan starts exactly at this match's end (an accepted empty
		// match at that position is impossible thanks to the rule above).
		g.pos = endByte + 1
		if len(caps) > 0 {
			return caps
		}
		return []Value{g.ms.src[startByte-1 : endByte]}
	}
	return nil
}

// PatternGSub implements string.gsub. repl is the substitution source
// (string, *Table, or *GoFunc/*Closure); the function form is called
// back through callFn (the caller injects vm.CallValue to keep this
// file VM-API-free). maxN <= 0 means "unlimited".
func PatternGSub(
	s, pat string, repl Value, maxN int,
	callFn func(fn Value, args []Value) []Value,
) (string, int) {
	// A negative maxN is the "absent 4th argument" sentinel → unlimited.
	// An explicit 0 must perform no substitutions (callers clamp explicit
	// negatives to 0 before reaching here).
	if maxN < 0 {
		maxN = 1<<31 - 1
	}
	// Lua accepts a number repl and treats it as its string form; anything
	// other than string/number/table/function is a bad argument.
	switch repl.(type) {
	case int64, float64:
		repl = ToString(repl)
	case string, *Table, *Closure, *GoFunc:
	default:
		panic(Errorf("bad argument #3 to 'gsub' (string/function/table expected, got %s)", TypeName(repl)))
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
	// Lua 5.4 semantics: an empty match whose end coincides with the end of
	// the previous match is rejected (`e != lastmatch` in str_gsub), so
	// gsub("abc", "a*", "X") is "XbXcX" (3), not the 5.3-era "XXbXcX" (4).
	lastMatch := -1
	ms := matchState{src: s, pat: pat, captures: make([]patternCapture, 0, 4)}
	for count < maxN {
		ms.captures = ms.captures[:0]
		end := ms.match(srcPos, pStart)
		if end >= 0 && end != lastMatch {
			caps := ms.collectCaptures(srcPos, end)
			matchStr := s[srcPos:end]
			applyReplacement(&b, matchStr, caps, repl, callFn)
			count++
			lastMatch = end
		}
		if end > srcPos {
			// Non-empty match: continue right after it.
			srcPos = end
		} else if srcPos < len(s) {
			// No match, or an empty one: emit one source byte to progress.
			b.WriteByte(s[srcPos])
			srcPos++
		} else {
			break
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
			if c != '%' {
				b.WriteByte(c)
				continue
			}
			if i+1 >= len(r) {
				panic(Errorf("invalid use of '%%' in replacement string"))
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
				switch {
				case idx < len(caps):
					b.WriteString(ToString(caps[idx]))
				case idx == 0:
					// %1 with no explicit captures aliases the whole match.
					b.WriteString(matchStr)
				default:
					panic(Errorf("invalid capture index %%%d in replacement string", idx+1))
				}
			default:
				panic(Errorf("invalid use of '%%' in replacement string"))
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
		writeSubstValue(b, matchStr, r.Get(key))
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
		writeSubstValue(b, matchStr, v)
	}
}

// writeSubstValue writes the value produced by a table/function replacement.
// Lua 5.4: nil/false keeps the original match, strings and numbers are
// substituted, anything else is an error.
func writeSubstValue(b *strings.Builder, matchStr string, v Value) {
	switch x := v.(type) {
	case nil:
		b.WriteString(matchStr)
	case bool:
		if x {
			panic(Errorf("invalid replacement value (a boolean)"))
		}
		b.WriteString(matchStr)
	case string:
		b.WriteString(x)
	case int64, float64:
		b.WriteString(ToString(v))
	default:
		panic(Errorf("invalid replacement value (a %s)", TypeName(v)))
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
			// Open capture at match end — the pattern is malformed.
			panic(Errorf("unfinished capture"))
		default:
			out[i] = ms.src[c.start : c.start+c.len]
		}
	}
	_ = sStart
	return out
}

// Core matcher

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
			panic(Errorf("malformed pattern (ends with '%%')"))
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
			panic(Errorf("malformed pattern (missing ']')"))
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
	// A ']' immediately after '[' or '[^' is a literal member, not the close
	// bracket (matches singleClassEnd's length detection and Lua semantics).
	first := true
	for idx < len(ms.pat) && (first || ms.pat[idx] != ']') {
		first = false
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
		panic(Errorf("invalid pattern capture"))
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
		panic(Errorf("invalid capture index %%%d in pattern", n+1))
	}
	c := ms.captures[n]
	if c.len < 0 {
		panic(Errorf("unfinished capture"))
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
		panic(Errorf("malformed pattern (missing arguments to '%%b')"))
	}
	open, close := ms.pat[pIdx], ms.pat[pIdx+1]
	if sIdx >= len(ms.src) || ms.src[sIdx] != open {
		return -1
	}
	depth := 1
	i := sIdx + 1
	for i < len(ms.src) {
		// Test close before open so a pattern with identical delimiters
		// (e.g. %b||) still terminates, matching Lua's matchbalance.
		if ms.src[i] == close {
			depth--
			if depth == 0 {
				return ms.match(i+1, pIdx+2)
			}
		} else if ms.src[i] == open {
			depth++
		}
		i++
	}
	return -1
}

// matchFrontier handles `%f[set]` — a zero-width assertion: the previous
// source char must NOT be in [set] and the current one must.
func (ms *matchState) matchFrontier(sIdx, pIdx int) int {
	if pIdx >= len(ms.pat) || ms.pat[pIdx] != '[' {
		panic(Errorf("missing '[' after '%%f' in pattern"))
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
