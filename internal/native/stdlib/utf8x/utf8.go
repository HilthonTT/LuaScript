// Package utf8x implements Lua 5.4's utf8 library. The package directory is
// `utf8x` (not `utf8`) so it does not clash with Go's standard `unicode/utf8`
// when both are imported; the module is exposed to Lua as `utf8` via the
// preload registrar.
package utf8x

import (
	"strings"
	"unicode/utf8"

	"github.com/hilthontt/luascript/internal/vm"
)

// Lua's utf8.charpattern — matches one byte sequence of a UTF-8 character.
// Same value as in PUC Lua 5.4.
const charpattern = "[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"

// RegisterUTF8Preload installs the utf8 module at package.preload.
func RegisterUTF8Preload(v *vm.VM) {
	vm.RegisterPreload(v, "utf8", loader)
}

func loader(_ *vm.VM, _ []vm.Value) []vm.Value {
	return []vm.Value{newUTF8()}
}

func newUTF8() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 6)
	add := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "utf8." + name, Fn: fn})
	}

	add("char", func(_ *vm.VM, args []vm.Value) []vm.Value {
		var b strings.Builder
		for i, a := range args {
			n, ok := vm.ToInteger(a)
			if !ok {
				panic(vm.Errorf("bad argument #%d to 'char' (number expected)", i+1))
			}
			if n < 0 || n > 0x7FFFFFFF {
				panic(vm.Errorf("bad argument #%d to 'char' (value %d out of range)", i+1, n))
			}
			// Encoded by hand rather than via WriteRune: WriteRune replaces
			// surrogates (U+D800..U+DFFF) and anything above U+10FFFF with
			// U+FFFD, so utf8.char(0xD800) silently produced a different
			// string than it was asked for. Lua 5.4 encodes the full 31-bit
			// range verbatim, surrogates included.
			b.Write(encodeLua(uint32(n)))
		}
		return []vm.Value{b.String()}
	})

	add("codepoint", func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("codepoint", 1, args)
		i := int64(1)
		if len(args) >= 2 {
			i = vm.IntArg("codepoint", 2, args)
		}
		j := i
		if len(args) >= 3 {
			j = vm.IntArg("codepoint", 3, args)
		}
		bi, bj := luaRange(s, i, j)
		var out []vm.Value
		idx := bi
		for idx < bj {
			r, sz := utf8.DecodeRuneInString(s[idx:])
			if r == utf8.RuneError && sz <= 1 {
				panic(vm.Errorf("invalid UTF-8 code at byte %d", idx+1))
			}
			out = append(out, int64(r))
			idx += sz
		}
		return out
	})

	add("len", func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("len", 1, args)
		i := int64(1)
		if len(args) >= 2 {
			i = vm.IntArg("len", 2, args)
		}
		j := int64(-1)
		if len(args) >= 3 {
			j = vm.IntArg("len", 3, args)
		}
		bi, bj := luaRange(s, i, j)
		count := int64(0)
		idx := bi
		for idx < bj {
			r, sz := utf8.DecodeRuneInString(s[idx:])
			if r == utf8.RuneError && sz <= 1 {
				// Lua returns (fail, byte position of bad char).
				return []vm.Value{nil, int64(idx + 1)}
			}
			count++
			idx += sz
		}
		return []vm.Value{count}
	})

	add("offset", func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("offset", 1, args)
		n := vm.IntArg("offset", 2, args)
		// Default i: 1 when n>=0, len(s)+1 when n<0.
		var i int64
		if n >= 0 {
			i = 1
		} else {
			i = int64(len(s)) + 1
		}
		if len(args) >= 3 {
			i = vm.IntArg("offset", 3, args)
		}
		idx := luaPos1(s, i) // byte index (0-based)
		if idx < 0 || idx > len(s) {
			panic(vm.Errorf("bad argument #3 to 'offset' (position out of bounds)"))
		}
		switch {
		case n > 0:
			n--
			for n > 0 && idx < len(s) {
				_, sz := utf8.DecodeRuneInString(s[idx:])
				idx += sz
				n--
			}
			if n > 0 {
				return []vm.Value{nil}
			}
		case n < 0:
			for n < 0 && idx > 0 {
				// Step back to the previous rune boundary.
				idx--
				for idx > 0 && (s[idx]&0xC0) == 0x80 {
					idx--
				}
				n++
			}
			if n < 0 {
				return []vm.Value{nil}
			}
		default:
			// n == 0 (Lua 5.4 special case): return the start of the
			// character encoding that contains byte i.
			for idx > 0 && idx < len(s) && (s[idx]&0xC0) == 0x80 {
				idx--
			}
		}
		return []vm.Value{int64(idx + 1)}
	})

	add("codes", func(_ *vm.VM, args []vm.Value) []vm.Value {
		s := vm.StringArg("codes", 1, args)
		iter := &vm.GoFunc{Name: "utf8:codes:iter", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
			cur := int64(0)
			if len(a) >= 2 {
				if n, ok := vm.ToInteger(a[1]); ok {
					cur = n
				}
			}
			// cur is the 1-based position of the LAST rune produced; 0 means
			// "start at byte 0". Advance by that rune's byte size.
			idx := 0
			if cur > 0 {
				idx = int(cur - 1)
				if idx < len(s) {
					_, sz := utf8.DecodeRuneInString(s[idx:])
					idx += sz
				}
			}
			if idx >= len(s) {
				return []vm.Value{nil}
			}
			r, sz := utf8.DecodeRuneInString(s[idx:])
			if r == utf8.RuneError && sz <= 1 {
				panic(vm.Errorf("invalid UTF-8 code at byte %d", idx+1))
			}
			_ = sz
			return []vm.Value{int64(idx + 1), int64(r)}
		}}
		return []vm.Value{iter, s, int64(0)}
	})

	m.Set("charpattern", charpattern)

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// encodeLua encodes one code point the way PUC Lua's utf8_esc / luaO_utf8esc
// does: the standard UTF-8 algorithm extended to six bytes so the whole 31-bit
// range round-trips, with no surrogate or U+10FFFF ceiling. Go's utf8 package
// deliberately refuses both, which is correct for Go strings and wrong for a
// Lua utf8 library.
func encodeLua(r uint32) []byte {
	switch {
	case r < 0x80:
		return []byte{byte(r)}
	case r < 0x800:
		return []byte{byte(0xC0 | r>>6), byte(0x80 | r&0x3F)}
	case r < 0x10000:
		return []byte{byte(0xE0 | r>>12), byte(0x80 | r>>6&0x3F), byte(0x80 | r&0x3F)}
	case r < 0x200000:
		return []byte{byte(0xF0 | r>>18), byte(0x80 | r>>12&0x3F),
			byte(0x80 | r>>6&0x3F), byte(0x80 | r&0x3F)}
	case r < 0x4000000:
		return []byte{byte(0xF8 | r>>24), byte(0x80 | r>>18&0x3F), byte(0x80 | r>>12&0x3F),
			byte(0x80 | r>>6&0x3F), byte(0x80 | r&0x3F)}
	default:
		return []byte{byte(0xFC | r>>30), byte(0x80 | r>>24&0x3F), byte(0x80 | r>>18&0x3F),
			byte(0x80 | r>>12&0x3F), byte(0x80 | r>>6&0x3F), byte(0x80 | r&0x3F)}
	}
}

// luaRange converts Lua-style (i, j) 1-based byte indices to Go 0-based
// half-open [bi, bj). Negative values count from the end. Out-of-range
// values are clamped to valid bounds rather than raising.
func luaRange(s string, i, j int64) (int, int) {
	n := int64(len(s))
	if i < 0 {
		i = n + 1 + i
	}
	if j < 0 {
		j = n + 1 + j
	}
	if i < 1 {
		i = 1
	}
	if j > n {
		j = n
	}
	if i > j {
		return 0, 0
	}
	return int(i - 1), int(j)
}

func luaPos1(s string, i int64) int {
	n := int64(len(s))
	if i < 0 {
		i = n + 1 + i
	}
	return int(i - 1)
}
