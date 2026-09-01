package vm

import (
	"strings"
)

const (
	patternMaxCaptures = 32
	patternMaxDepth    = 200
	capPosition        = -1
	capUnfinished      = -2
)

type patternCapture struct {
	start int
	len   int
}

type matchState struct {
	src      string
	pat      string
	captures []patternCapture
	depth    int
}

func PatternHasSpecials(pat string) bool {
	const magic = "^$*+?.([%-"
	return strings.ContainsAny(pat, magic)
}

func PatternFind(s, pat string, init int) (int, int, []Value, bool) {
	ms := matchState{src: s, pat: pat, captures: make([]patternCapture, 0, 4)}
	return ms.find(init)
}

func (ms *matchState) find(init int) (int, int, []Value, bool) {
	return ms.findImpl(init, true)
}

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

type GMatchIter struct {
	ms        matchState
	pos       int
	lastMatch int
}

func NewGMatchIter(s, pat string, init int) *GMatchIter {
	return &GMatchIter{
		ms:        matchState{src: s, pat: pat, captures: make([]patternCapture, 0, 4)},
		pos:       luaInit(s, init) + 1,
		lastMatch: -1,
	}
}

func (g *GMatchIter) Next() []Value {
	for g.pos <= len(g.ms.src)+1 {
		startByte, endByte, caps, ok := g.ms.findImpl(g.pos, false)
		if !ok {
			return nil
		}
		if endByte == g.lastMatch && endByte == startByte-1 {
			g.pos = startByte + 1
			continue
		}
		g.lastMatch = endByte
		g.pos = endByte + 1
		if len(caps) > 0 {
			return caps
		}
		return []Value{g.ms.src[startByte-1 : endByte]}
	}
	return nil
}

func PatternGSub(
	s, pat string, repl Value, maxN int,
	callFn func(fn Value, args []Value) []Value,
) (string, int) {
	if maxN < 0 {
		maxN = 1<<31 - 1
	}
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
			srcPos = end
		} else if srcPos < len(s) {
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

func applyReplacement(
	b *strings.Builder, matchStr string, caps []Value,
	repl Value, callFn func(fn Value, args []Value) []Value,
) {
	switch r := repl.(type) {
	case string:
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
					b.WriteString(matchStr)
				default:
					panic(Errorf("invalid capture index %%%d in replacement string", idx+1))
				}
			default:
				panic(Errorf("invalid use of '%%' in replacement string"))
			}
		}
	case *Table:
		var key Value
		if len(caps) > 0 {
			key = caps[0]
		} else {
			key = matchStr
		}
		writeSubstValue(b, matchStr, r.Get(key))
	default:
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

func luaInit(s string, init int) int {
	if init < 0 {
		init = len(s) + 1 + init
	}
	if init < 1 {
		init = 1
	}
	return init - 1
}

func (ms *matchState) collectCaptures(sStart, matchEnd int) []Value {
	_ = matchEnd
	if len(ms.captures) == 0 {
		return nil
	}
	out := make([]Value, len(ms.captures))
	for i, c := range ms.captures {
		switch c.len {
		case capPosition:
			out[i] = int64(c.start + 1)
		case capUnfinished:
			panic(Errorf("unfinished capture"))
		default:
			out[i] = ms.src[c.start : c.start+c.len]
		}
	}
	_ = sStart
	return out
}

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
			if pIdx+1 < len(ms.pat) && ms.pat[pIdx+1] == ')' {
				return ms.startCapture(sIdx, pIdx+2, capPosition)
			}
			return ms.startCapture(sIdx, pIdx+1, capUnfinished)
		case ')':
			return ms.closeCapture(sIdx, pIdx+1)
		case '$':
			if pIdx+1 == len(ms.pat) {
				if sIdx == len(ms.src) {
					return sIdx
				}
				return -1
			}
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
		if end < len(ms.pat) && ms.pat[end] == ']' {
			end++
		}
		for end < len(ms.pat) && ms.pat[end] != ']' {
			if ms.pat[end] == '%' && end+1 < len(ms.pat) {
				end++
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

func (ms *matchState) matchBracket(c byte, pIdx int) bool {
	idx := pIdx + 1
	negate := false
	if idx < len(ms.pat) && ms.pat[idx] == '^' {
		negate = true
		idx++
	}
	found := false
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
