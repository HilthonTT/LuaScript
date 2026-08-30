package vm

// The baseline `string` library, including string.format.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// string

func buildStringLibrary() *Table {
	t := NewTable(0, 16)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "string." + name, Fn: fn})
	}

	add("len", func(_ *VM, args []Value) []Value {
		s := StringArg("string.len", 1, args)
		return []Value{int64(len(s))}
	})
	add("upper", func(_ *VM, args []Value) []Value {
		s := StringArg("string.upper", 1, args)
		// Byte-wise ASCII case mapping, like Lua's C-locale toupper.
		// Unicode-aware mapping would rewrite (and can resize) non-ASCII
		// bytes, breaking byte-oriented string code.
		return []Value{asciiMapCase(s, 'a', 'z', 'A'-'a')}
	})
	add("lower", func(_ *VM, args []Value) []Value {
		s := StringArg("string.lower", 1, args)
		return []Value{asciiMapCase(s, 'A', 'Z', 'a'-'A')}
	})
	add("reverse", func(_ *VM, args []Value) []Value {
		s := StringArg("string.reverse", 1, args)
		// Byte-reverse (Lua treats strings as byte sequences).
		b := []byte(s)
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		return []Value{string(b)}
	})
	add("rep", func(_ *VM, args []Value) []Value {
		s := StringArg("string.rep", 1, args)
		n := IntArg("string.rep", 2, args)
		if n <= 0 {
			return []Value{""}
		}
		sep := ""
		if len(args) >= 3 {
			sep = StringArg("string.rep", 3, args)
		}
		// Bound the result so a script-chosen count can't demand an
		// allocation big enough to OOM the process (fatal, not
		// pcall-catchable). Mirrors reference Lua's "resulting string too
		// large" error.
		const maxRepLen = 256 * 1024 * 1024
		unit := int64(len(s) + len(sep))
		if unit == 0 {
			return []Value{""}
		}
		if n > maxRepLen/unit {
			panic(Errorf("string.rep: resulting string too large"))
		}
		if sep == "" {
			return []Value{strings.Repeat(s, int(n))}
		}
		// (s..sep) * (n-1) + s. Built directly rather than via a []string +
		// strings.Join: the slice would cost 16 bytes of header per element,
		// letting string.rep("", huge, "x") amplify 16x past the byte guard
		// above into a fatal OOM.
		var b strings.Builder
		b.Grow(int(unit*n) - len(sep))
		for i := int64(0); i < n; i++ {
			if i > 0 {
				b.WriteString(sep)
			}
			b.WriteString(s)
		}
		return []Value{b.String()}
	})
	add("sub", func(_ *VM, args []Value) []Value {
		s := StringArg("string.sub", 1, args)
		i := IntArg("string.sub", 2, args)
		j := int64(len(s))
		if len(args) >= 3 {
			j = IntArg("string.sub", 3, args)
		}
		// Lua 1-based, negative indices count from the end.
		ln := int64(len(s))
		if i < 0 {
			i = ln + i + 1
		}
		if j < 0 {
			j = ln + j + 1
		}
		if i < 1 {
			i = 1
		}
		if j > ln {
			j = ln
		}
		if i > j {
			return []Value{""}
		}
		return []Value{s[i-1 : j]}
	})
	add("byte", func(_ *VM, args []Value) []Value {
		s := StringArg("string.byte", 1, args)
		ln := int64(len(s))
		i := OptInt("string.byte", 2, args, 1)
		j := OptInt("string.byte", 3, args, i)
		// Lua 5.4: negative indices count from the end; the range is then
		// clamped to the string and an empty range yields no values.
		if i < 0 {
			i = ln + i + 1
		}
		if j < 0 {
			j = ln + j + 1
		}
		if i < 1 {
			i = 1
		}
		if j > ln {
			j = ln
		}
		if i > j {
			return nil
		}
		out := make([]Value, 0, j-i+1)
		for k := i; k <= j; k++ {
			out = append(out, int64(s[k-1]))
		}
		return out
	})
	add("char", func(_ *VM, args []Value) []Value {
		buf := make([]byte, len(args))
		for i, a := range args {
			b, ok := ToInteger(a)
			if !ok || b < 0 || b > 255 {
				panic(Errorf("bad argument #%d to 'string.char' (value out of range)", i+1))
			}
			buf[i] = byte(b)
		}
		return []Value{string(buf)}
	})
	add("find", func(_ *VM, args []Value) []Value {
		// Lua semantics: returns startPos, endPos[, captures...]. The
		// optional 4th `plain` arg bypasses pattern matching; otherwise
		// any magic character engages the pattern engine. Plain-only
		// patterns auto-fast-path through strings.Index for performance.
		s := StringArg("string.find", 1, args)
		pat := StringArg("string.find", 2, args)
		init := int64(1)
		if len(args) >= 3 {
			if n, ok := args[2].(int64); ok {
				init = n
			} else if n, ok := ToInteger(args[2]); ok {
				init = n
			}
		}
		plain := false
		if len(args) >= 4 {
			plain = IsTruthy(args[3])
		}
		if !plain && PatternHasSpecials(pat) {
			startByte, endByte, caps, ok := PatternFind(s, pat, int(init))
			if !ok {
				return []Value{nil}
			}
			out := make([]Value, 0, 2+len(caps))
			out = append(out, int64(startByte), int64(endByte))
			out = append(out, caps...)
			return out
		}
		ln := int64(len(s))
		if init < 0 {
			init = ln + init + 1
		}
		if init < 1 {
			init = 1
		}
		if init > ln+1 {
			return []Value{nil}
		}
		idx := strings.Index(s[init-1:], pat)
		if idx < 0 {
			return []Value{nil}
		}
		startPos := init + int64(idx)
		endPos := startPos + int64(len(pat)) - 1
		return []Value{startPos, endPos}
	})
	add("match", func(_ *VM, args []Value) []Value {
		s := StringArg("string.match", 1, args)
		pat := StringArg("string.match", 2, args)
		init := int64(1)
		if len(args) >= 3 {
			if n, ok := ToInteger(args[2]); ok {
				init = n
			}
		}
		return PatternMatch(s, pat, int(init))
	})
	add("gmatch", func(_ *VM, args []Value) []Value {
		s := StringArg("string.gmatch", 1, args)
		pat := StringArg("string.gmatch", 2, args)
		init := int(OptInt("string.gmatch", 3, args, 1))
		it := NewGMatchIter(s, pat, init)
		iter := &GoFunc{Name: "string:gmatch:iter", Fn: func(_ *VM, _ []Value) []Value {
			r := it.Next()
			if r == nil {
				return []Value{nil}
			}
			return r
		}}
		return []Value{iter}
	})
	add("gsub", func(v *VM, args []Value) []Value {
		s := StringArg("string.gsub", 1, args)
		pat := StringArg("string.gsub", 2, args)
		if len(args) < 3 {
			panic(Errorf("bad argument #3 to 'string.gsub' (string/table/function expected)"))
		}
		repl := args[2]
		n := -1 // sentinel: 4th arg absent → replace all
		if len(args) >= 4 {
			if i, ok := ToInteger(args[3]); ok {
				n = int(i)
				if n < 0 {
					n = 0 // Lua: a non-positive count performs no substitutions
				}
			}
		}
		out, count := PatternGSub(s, pat, repl, n, func(fn Value, fnArgs []Value) []Value {
			return v.CallValue(fn, fnArgs, -1)
		})
		return []Value{out, int64(count)}
	})
	add("format", func(v *VM, args []Value) []Value {
		fmtStr := StringArg("string.format", 1, args)
		return []Value{luaFormat(v, fmtStr, args[1:])}
	})
	// Binary (de)serialization — implemented in strpack.go.
	add("pack", builtinStringPack)
	add("unpack", builtinStringUnpack)
	add("packsize", builtinStringPacksize)
	return t
}

// asciiMapCase shifts bytes in [lo, hi] by delta, leaving every other byte
// (including non-ASCII) untouched.
func asciiMapCase(s string, lo, hi byte, delta int) string {
	b := []byte(s)
	changed := false
	for i, c := range b {
		if c >= lo && c <= hi {
			b[i] = byte(int(c) + delta)
			changed = true
		}
	}
	if !changed {
		return s
	}
	return string(b)
}

// luaFormat implements string.format per Lua 5.4: each %-directive is parsed,
// its argument coerced to the type the verb expects (integers for diouxXc,
// floats for aAeEfgG, strings via tostring for s), then rendered through Go's
// fmt with an equivalent verb. This replaces the old thin fmt.Sprintf wrapper,
// which leaked Go's stricter verb typing (e.g. %d rejecting a float 3.0).
func luaFormat(v *VM, format string, args []Value) string {
	var b strings.Builder
	argIdx := 0
	nextArg := func() Value {
		if argIdx >= len(args) {
			panic(Errorf("bad argument #%d to 'string.format' (no value)", argIdx+2))
		}
		v := args[argIdx]
		argIdx++
		return v
	}

	i := 0
	for i < len(format) {
		if format[i] != '%' {
			b.WriteByte(format[i])
			i++
			continue
		}
		// Parse a directive: %[-+ #0]*[width][.precision]verb
		j := i + 1
		if j < len(format) && format[j] == '%' {
			b.WriteByte('%')
			i = j + 1
			continue
		}
		for j < len(format) && strings.IndexByte("-+ #0", format[j]) >= 0 {
			j++
		}
		for j < len(format) && format[j] >= '0' && format[j] <= '9' {
			j++
		}
		if j < len(format) && format[j] == '.' {
			j++
			for j < len(format) && format[j] >= '0' && format[j] <= '9' {
				j++
			}
		}
		if j >= len(format) {
			panic(Errorf("invalid conversion '%s' to 'string.format'", format[i:]))
		}
		verb := format[j]
		spec := format[i : j+1] // the whole directive, e.g. "%5.2d"
		i = j + 1

		switch verb {
		case 'd', 'i', 'u':
			b.WriteString(fmt.Sprintf(replaceVerb(spec, 'd'), fmtArgInt(nextArg())))
		case 'o', 'x', 'X':
			// C (and Lua) render these as the unsigned 64-bit bit pattern;
			// Go would print a sign instead ("%x" of -1 → "-1", not "f…f").
			b.WriteString(fmt.Sprintf(spec, uint64(fmtArgInt(nextArg()))))
		case 'c':
			// Render the byte through %s so width/flags apply ("%5c").
			b.WriteString(fmt.Sprintf(replaceVerb(spec, 's'),
				string(byte(fmtArgInt(nextArg())))))
		case 'g', 'G':
			// C's %g defaults to 6 significant digits; Go's default is
			// "shortest unique", so make the Lua default explicit.
			if !strings.Contains(spec, ".") {
				spec = spec[:len(spec)-1] + ".6" + string(verb)
			}
			b.WriteString(fmt.Sprintf(spec, fmtArgFloat(nextArg())))
		case 'e', 'E', 'f', 'F':
			b.WriteString(fmt.Sprintf(spec, fmtArgFloat(nextArg())))
		case 'a':
			b.WriteString(fmt.Sprintf(replaceVerb(spec, 'x'), fmtArgFloat(nextArg())))
		case 'A':
			b.WriteString(fmt.Sprintf(replaceVerb(spec, 'X'), fmtArgFloat(nextArg())))
		case 's':
			b.WriteString(fmt.Sprintf(spec, ToStringMM(v, nextArg())))
		case 'q':
			b.WriteString(formatQ(nextArg()))
		default:
			panic(Errorf("invalid conversion '%%%c' to 'string.format'", verb))
		}
	}
	return b.String()
}

// replaceVerb swaps the trailing conversion verb of a directive.
func replaceVerb(spec string, verb byte) string {
	return spec[:len(spec)-1] + string(verb)
}

// fmtArgInt coerces a format argument to an integer the way Lua's %d does.
func fmtArgInt(v Value) int64 {
	if x, ok := v.(float64); ok {
		if n, ok2 := floatToInt(x); ok2 {
			return n
		}
		panic(Errorf("bad argument to 'string.format' (number has no integer representation)"))
	}
	if n, ok := ToInteger(v); ok {
		return n
	}
	panic(Errorf("bad argument to 'string.format' (number expected, got %s)", TypeName(v)))
}

// fmtArgFloat coerces a format argument to a float for the %f/%g/%e family.
func fmtArgFloat(v Value) float64 {
	if f, ok := ToFloat(v); ok {
		return f
	}
	panic(Errorf("bad argument to 'string.format' (number expected, got %s)", TypeName(v)))
}

// formatQ renders a value as a literal that reads back losslessly, per
// Lua 5.4: strings get byte-level \ddd escapes (never Go's \uXXXX, which
// isn't Lua syntax), floats print as hex floats to preserve both value and
// float-ness, mininteger prints in hex (its decimal form reads back as a
// float), and inf/nan use the conventional 1e9999 / (0/0) spellings.
func formatQ(v Value) string {
	switch x := v.(type) {
	case string:
		var b strings.Builder
		b.Grow(len(x) + 2)
		b.WriteByte('"')
		for i := 0; i < len(x); i++ {
			c := x[i]
			switch {
			case c == '"' || c == '\\' || c == '\n':
				b.WriteByte('\\')
				b.WriteByte(c)
			case c < 32 || c >= 127:
				// \ddd, zero-padded only when the next byte is a digit
				// (so the escape isn't extended by it). Bytes >= 128 are
				// escaped too (PUC Lua passes them raw): our lexer decodes
				// source as UTF-8 runes, so a raw non-UTF-8 byte would be
				// mangled to U+FFFD on load. Escaping keeps %q ASCII-safe.
				if i+1 < len(x) && x[i+1] >= '0' && x[i+1] <= '9' {
					fmt.Fprintf(&b, "\\%03d", c)
				} else {
					fmt.Fprintf(&b, "\\%d", c)
				}
			default:
				b.WriteByte(c)
			}
		}
		b.WriteByte('"')
		return b.String()
	case int64:
		if x == math.MinInt64 {
			return "0x8000000000000000"
		}
		return strconv.FormatInt(x, 10)
	case float64:
		if math.IsInf(x, 1) {
			return "1e9999"
		}
		if math.IsInf(x, -1) {
			return "-1e9999"
		}
		if math.IsNaN(x) {
			return "(0/0)"
		}
		return strconv.FormatFloat(x, 'x', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return "nil"
	}
	panic(Errorf("bad argument to 'string.format' (value has no literal form)"))
}
