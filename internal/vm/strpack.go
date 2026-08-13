package vm

// string.pack / string.unpack / string.packsize — Lua 5.4's binary
// serialization primitives, used for reading and writing fixed binary layouts
// (file headers, network frames, C structs) without a byte-at-a-time loop in
// script code.
//
// The format language and the option letters follow the reference manual, and
// the implementation mirrors lstrlib.c's structure (getdetails/packint/
// unpackint) so the alignment and overflow rules match rather than approximate.
//
// One deliberate divergence: PUC Lua sizes 'l'/'L' with C's sizeof(long), which
// is 8 on Unix and 4 on Windows. Sizing a serialization format off the host
// compiler makes a packed file unreadable on the other platform, so this
// runtime fixes 'l'/'L' at 8 bytes everywhere. Scripts that want an explicit
// width should use i4/I4 or i8/I8, which are unambiguous on every platform.

import (
	"fmt"
	"math"
	"strings"
)

// kind enumerates what one parsed format option asks for.
type kind int

const (
	kInt       kind = iota // signed integer of `size` bytes
	kUint                  // unsigned integer of `size` bytes
	kFloat                 // 4-byte IEEE float
	kDouble                // 8-byte IEEE double
	kChar                  // fixed-width string of `size` bytes
	kString                // string prefixed by a `size`-byte length
	kZeroStr               // NUL-terminated string
	kPadding               // one zero byte
	kPaddAlign             // alignment-only, takes its size from the next option
	kNop                   // whitespace and endianness/alignment directives
)

// maxIntSize bounds i/I widths, matching Lua's 16-byte ceiling.
const maxIntSize = 16

// packHeader carries the state the endianness and alignment directives mutate
// as the format string is walked left to right.
type packHeader struct {
	little   bool // '<' / '>' / '=' — native is little on every supported target
	maxAlign int  // '!' raises this; 1 (no alignment) until it does
	fmt      string
	pos      int
	site     string // "string.pack" etc., for error messages
}

func newPackHeader(format, site string) *packHeader {
	return &packHeader{little: true, maxAlign: 1, fmt: format, site: site}
}

func (h *packHeader) errf(msg string, args ...any) {
	panic(Errorf("bad argument #1 to '%s' (%s)", h.site, fmt.Sprintf(msg, args...)))
}

// nextDigits reads an optional decimal size suffix, returning dflt when the
// next character is not a digit.
func (h *packHeader) nextDigits(dflt int) int {
	if h.pos >= len(h.fmt) || h.fmt[h.pos] < '0' || h.fmt[h.pos] > '9' {
		return dflt
	}
	n := 0
	for h.pos < len(h.fmt) && h.fmt[h.pos] >= '0' && h.fmt[h.pos] <= '9' {
		n = n*10 + int(h.fmt[h.pos]-'0')
		if n > 0xFFFF { // far past any legal size; stop before overflowing
			h.errf("integral size out of limits")
		}
		h.pos++
	}
	return n
}

// sizedInt reads the width for i/I and s, rejecting out-of-range values.
func (h *packHeader) sizedInt(dflt int) int {
	n := h.nextDigits(dflt)
	if n < 1 || n > maxIntSize {
		h.errf("integral size (%d) out of limits [1,%d]", n, maxIntSize)
	}
	return n
}

// nextOption parses one option, returning its kind and byte size. ok is false
// at end of format.
func (h *packHeader) nextOption() (k kind, size int, ok bool) {
	if h.pos >= len(h.fmt) {
		return kNop, 0, false
	}
	c := h.fmt[h.pos]
	h.pos++
	switch c {
	case 'b':
		return kInt, 1, true
	case 'B':
		return kUint, 1, true
	case 'h':
		return kInt, 2, true
	case 'H':
		return kUint, 2, true
	case 'l', 'j':
		return kInt, 8, true
	case 'L', 'J', 'T':
		return kUint, 8, true
	case 'i':
		return kInt, h.sizedInt(4), true
	case 'I':
		return kUint, h.sizedInt(4), true
	case 'f':
		return kFloat, 4, true
	case 'd', 'n':
		return kDouble, 8, true
	case 's':
		return kString, h.sizedInt(8), true
	case 'z':
		return kZeroStr, 0, true
	case 'x':
		return kPadding, 1, true
	case 'X':
		return kPaddAlign, 0, true
	case 'c':
		n := h.nextDigits(-1)
		if n < 0 {
			h.errf("missing size for format option 'c'")
		}
		return kChar, n, true
	case '<':
		h.little = true
		return kNop, 0, true
	case '>':
		h.little = false
		return kNop, 0, true
	case '=':
		h.little = true // native: every target this runtime builds for
		return kNop, 0, true
	case '!':
		h.maxAlign = h.sizedInt(8)
		return kNop, 0, true
	case ' ':
		return kNop, 0, true
	default:
		h.errf("invalid format option '%c'", c)
		return kNop, 0, false
	}
}

// details parses the next option and computes how many padding bytes must
// precede it to satisfy the current alignment. Mirrors lstrlib.c's getdetails.
func (h *packHeader) details(totalSize int) (k kind, size, toAlign int, ok bool) {
	k, size, ok = h.nextOption()
	if !ok {
		return
	}
	align := size
	if k == kPaddAlign {
		// 'X' carries no data of its own; it borrows the alignment of the
		// option that follows, which therefore must exist and must have one.
		var nk kind
		var nsize int
		var nok bool
		nk, nsize, nok = h.nextOption()
		if !nok || nk == kChar || nsize == 0 {
			h.errf("invalid next option for option 'X'")
		}
		align = nsize
	}
	if align <= 1 || k == kChar {
		return k, size, 0, true
	}
	if align > h.maxAlign {
		align = h.maxAlign
	}
	if align&(align-1) != 0 {
		h.errf("format asks for alignment not power of 2")
	}
	if align <= 1 {
		return k, size, 0, true
	}
	return k, size, (align - totalSize&(align-1)) & (align - 1), true
}

// packInt writes n as a `size`-byte integer in the header's byte order,
// sign-extending (or zero-extending) into any width beyond 8 bytes.
func packInt(b *strings.Builder, n uint64, little bool, size int, negative bool) {
	buf := make([]byte, size)
	limit := size
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		buf[i] = byte(n & 0xFF)
		n >>= 8
	}
	if negative {
		// Sign-extend a value narrower than the requested width.
		for i := limit; i < size; i++ {
			buf[i] = 0xFF
		}
	}
	if !little {
		for i, j := 0, size-1; i < j; i, j = i+1, j-1 {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}
	b.Write(buf)
}

// unpackInt reads a `size`-byte integer. Widths above 8 bytes are accepted
// only when the surplus bytes are pure sign extension, so a value that cannot
// be represented raises instead of being silently truncated.
func unpackInt(h *packHeader, data []byte, little bool, size int, signed bool) int64 {
	var n uint64
	if little {
		for i := size - 1; i >= 0; i-- {
			n = n<<8 | uint64(data[i])
		}
	} else {
		for i := 0; i < size; i++ {
			n = n<<8 | uint64(data[i])
		}
	}
	if size < 8 {
		if signed {
			// Sign-extend from the top bit of the packed width.
			shift := uint(64 - size*8)
			return int64(n<<shift) >> shift
		}
		return int64(n)
	}
	if size > 8 {
		// Bytes beyond the low 8 must all match the sign of what we kept.
		var fill byte
		if signed && n>>63 != 0 {
			fill = 0xFF
		}
		for i := 8; i < size; i++ {
			idx := i
			if !little {
				idx = size - 1 - i
			}
			if data[idx] != fill {
				h.errf("%d-byte integer does not fit into Lua Integer", size)
			}
		}
	}
	return int64(n)
}

func builtinStringPack(_ *VM, args []Value) []Value {
	format := StringArg("string.pack", 1, args)
	h := newPackHeader(format, "string.pack")

	var b strings.Builder
	argIdx := 1 // args[0] is the format
	nextArg := func() Value {
		if argIdx >= len(args) {
			panic(Errorf("bad argument #%d to 'string.pack' (no value)", argIdx+1))
		}
		v := args[argIdx]
		argIdx++
		return v
	}
	intArg := func() int64 {
		v := nextArg()
		n, ok := ToInteger(v)
		if !ok {
			panic(Errorf("bad argument #%d to 'string.pack' (number expected, got %s)", argIdx, describeBadArg(v)))
		}
		return n
	}
	strArg := func() string {
		v := nextArg()
		s, ok := v.(string)
		if !ok {
			panic(Errorf("bad argument #%d to 'string.pack' (string expected, got %s)", argIdx, describeBadArg(v)))
		}
		return s
	}

	for {
		k, size, toAlign, ok := h.details(b.Len())
		if !ok {
			break
		}
		for range toAlign {
			b.WriteByte(0)
		}
		switch k {
		case kNop, kPaddAlign:
			// No data of their own.
		case kPadding:
			b.WriteByte(0)
		case kInt, kUint:
			n := intArg()
			if size < 8 {
				// Reject values that would not survive the round trip rather
				// than writing truncated bytes the reader can't detect.
				if k == kInt {
					lim := int64(1) << (uint(size)*8 - 1)
					if n < -lim || n >= lim {
						panic(Errorf("bad argument #%d to 'string.pack' (integer overflow)", argIdx))
					}
				} else if n < 0 || uint64(n) >= uint64(1)<<(uint(size)*8) {
					panic(Errorf("bad argument #%d to 'string.pack' (unsigned overflow)", argIdx))
				}
			}
			packInt(&b, uint64(n), h.little, size, k == kInt && n < 0)
		case kFloat:
			f := nextArg()
			x, okf := ToFloat(f)
			if !okf {
				panic(Errorf("bad argument #%d to 'string.pack' (number expected, got %s)", argIdx, describeBadArg(f)))
			}
			packInt(&b, uint64(math.Float32bits(float32(x))), h.little, 4, false)
		case kDouble:
			f := nextArg()
			x, okf := ToFloat(f)
			if !okf {
				panic(Errorf("bad argument #%d to 'string.pack' (number expected, got %s)", argIdx, describeBadArg(f)))
			}
			packInt(&b, math.Float64bits(x), h.little, 8, false)
		case kChar:
			s := strArg()
			if len(s) > size {
				panic(Errorf("bad argument #%d to 'string.pack' (string longer than given size)", argIdx))
			}
			b.WriteString(s)
			for range size - len(s) {
				b.WriteByte(0)
			}
		case kString:
			s := strArg()
			if size < 8 && uint64(len(s)) >= uint64(1)<<(uint(size)*8) {
				panic(Errorf("bad argument #%d to 'string.pack' (string length does not fit in given size)", argIdx))
			}
			packInt(&b, uint64(len(s)), h.little, size, false)
			b.WriteString(s)
		case kZeroStr:
			s := strArg()
			if strings.IndexByte(s, 0) >= 0 {
				panic(Errorf("bad argument #%d to 'string.pack' (string contains zeros)", argIdx))
			}
			b.WriteString(s)
			b.WriteByte(0)
		}
	}
	return []Value{b.String()}
}

func builtinStringPacksize(_ *VM, args []Value) []Value {
	format := StringArg("string.packsize", 1, args)
	h := newPackHeader(format, "string.packsize")

	total := 0
	for {
		k, size, toAlign, ok := h.details(total)
		if !ok {
			break
		}
		switch k {
		case kString, kZeroStr:
			// Their width depends on the value, so the format has no fixed
			// size at all — the whole call is invalid, not just this option.
			h.errf("variable-size format in packsize")
		}
		total += toAlign + size
	}
	return []Value{int64(total)}
}

func builtinStringUnpack(_ *VM, args []Value) []Value {
	format := StringArg("string.unpack", 1, args)
	data := StringArg("string.unpack", 2, args)
	pos := int(OptInt("string.unpack", 3, args, 1))
	// Lua position: 1-based, negative counts from the end.
	if pos < 0 {
		pos = len(data) + pos + 1
	}
	if pos < 1 || pos > len(data)+1 {
		panic(Errorf("bad argument #3 to 'string.unpack' (initial position out of string)"))
	}
	idx := pos - 1

	h := newPackHeader(format, "string.unpack")
	var out []Value
	need := func(n int) []byte {
		if idx+n > len(data) {
			panic(Errorf("bad argument #2 to 'string.unpack' (data string too short)"))
		}
		b := []byte(data[idx : idx+n])
		idx += n
		return b
	}

	for {
		k, size, toAlign, ok := h.details(idx)
		if !ok {
			break
		}
		need(toAlign)
		switch k {
		case kNop, kPaddAlign:
		case kPadding:
			need(1)
		case kInt:
			out = append(out, unpackInt(h, need(size), h.little, size, true))
		case kUint:
			out = append(out, unpackInt(h, need(size), h.little, size, false))
		case kFloat:
			bits := uint32(unpackInt(h, need(4), h.little, 4, false))
			out = append(out, float64(math.Float32frombits(bits)))
		case kDouble:
			bits := uint64(unpackInt(h, need(8), h.little, 8, false))
			out = append(out, math.Float64frombits(bits))
		case kChar:
			out = append(out, string(need(size)))
		case kString:
			n := unpackInt(h, need(size), h.little, size, false)
			if n < 0 {
				panic(Errorf("bad argument #2 to 'string.unpack' (string length is negative)"))
			}
			out = append(out, string(need(int(n))))
		case kZeroStr:
			end := strings.IndexByte(data[idx:], 0)
			if end < 0 {
				panic(Errorf("bad argument #2 to 'string.unpack' (unfinished string for format 'z')"))
			}
			out = append(out, data[idx:idx+end])
			idx += end + 1
		}
	}
	// Lua returns the position of the first unread byte after the values.
	out = append(out, int64(idx+1))
	return out
}
