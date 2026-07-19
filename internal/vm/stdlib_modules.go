package vm

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
)

// registerLibraryModules installs the math / string / table / io modules.
// The string table is also wired in as the metatable's __index for string
// values so `("hi"):upper()` resolves to string.upper("hi").
func registerLibraryModules(v *VM) {
	v.Globals.Set("math", buildMathLibrary())
	stringLib := buildStringLibrary()
	v.Globals.Set("string", stringLib)
	v.Globals.Set("table", buildTableLibrary())
	v.Globals.Set("io", buildIOLibrary())

	// Strings carry a shared metatable whose __index is the `string` table.
	// This is what makes the colon-method syntax work on string values.
	v.stringMeta = NewTable(0, 1)
	v.stringMeta.Set("__index", stringLib)
}

// math

func buildMathLibrary() *Table {
	t := NewTable(0, 32)
	t.Set("pi", math.Pi)
	t.Set("huge", math.Inf(1))
	t.Set("maxinteger", int64(math.MaxInt64))
	t.Set("mininteger", int64(math.MinInt64))

	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "math." + name, Fn: fn})
	}

	add("abs", func(_ *VM, args []Value) []Value {
		i, f, isInt, ok := NumArg("math.abs", 1, args)
		if !ok {
			return []Value{nil}
		}
		if isInt {
			if i < 0 {
				return []Value{-i}
			}
			return []Value{i}
		}
		return []Value{math.Abs(f)}
	})
	add("ceil", func(_ *VM, args []Value) []Value {
		f := FloatArg("math.ceil", 1, args)
		r := math.Ceil(f)
		if i, ok := floatToInt(r); ok {
			return []Value{i}
		}
		return []Value{r}
	})
	add("floor", func(_ *VM, args []Value) []Value {
		f := FloatArg("math.floor", 1, args)
		r := math.Floor(f)
		if i, ok := floatToInt(r); ok {
			return []Value{i}
		}
		return []Value{r}
	})
	add("sqrt", floatToFloat1("math.sqrt", math.Sqrt))
	add("exp", floatToFloat1("math.exp", math.Exp))
	add("sin", floatToFloat1("math.sin", math.Sin))
	add("cos", floatToFloat1("math.cos", math.Cos))
	add("tan", floatToFloat1("math.tan", math.Tan))
	add("asin", floatToFloat1("math.asin", math.Asin))
	add("acos", floatToFloat1("math.acos", math.Acos))
	add("atan", func(_ *VM, args []Value) []Value {
		y := FloatArg("math.atan", 1, args)
		if len(args) >= 2 {
			x := FloatArg("math.atan", 2, args)
			return []Value{math.Atan2(y, x)}
		}
		return []Value{math.Atan(y)}
	})
	add("log", func(_ *VM, args []Value) []Value {
		x := FloatArg("math.log", 1, args)
		if len(args) >= 2 {
			b := FloatArg("math.log", 2, args)
			return []Value{math.Log(x) / math.Log(b)}
		}
		return []Value{math.Log(x)}
	})
	add("max", func(_ *VM, args []Value) []Value {
		best := AnyArg("max", 1, args)
		for _, a := range args[1:] {
			if mathLess(best, a) {
				best = a
			}
		}
		return []Value{best}
	})
	add("min", func(_ *VM, args []Value) []Value {
		best := AnyArg("min", 1, args)
		for _, a := range args[1:] {
			if mathLess(a, best) {
				best = a
			}
		}
		return []Value{best}
	})
	add("fmod", func(_ *VM, args []Value) []Value {
		// Two integer operands keep the integer subtype (truncated
		// remainder, sign of x — Go's % matches C fmod here).
		if len(args) >= 2 {
			if xi, okx := args[0].(int64); okx {
				if yi, oky := args[1].(int64); oky {
					if yi == 0 {
						panic(LuaError("bad argument #2 to 'fmod' (zero)"))
					}
					return []Value{xi % yi}
				}
			}
		}
		x := FloatArg("math.fmod", 1, args)
		y := FloatArg("math.fmod", 2, args)
		return []Value{math.Mod(x, y)}
	})
	add("modf", func(_ *VM, args []Value) []Value {
		x := FloatArg("math.modf", 1, args)
		intPart, fracPart := math.Modf(x)
		return []Value{intPart, fracPart}
	})
	add("pow", func(_ *VM, args []Value) []Value {
		x := FloatArg("math.pow", 1, args)
		y := FloatArg("math.pow", 2, args)
		return []Value{math.Pow(x, y)}
	})
	add("tointeger", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return []Value{nil}
		}
		if i, ok := ToInteger(args[0]); ok {
			return []Value{i}
		}
		return []Value{nil}
	})
	add("type", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return []Value{nil}
		}
		switch args[0].(type) {
		case int64:
			return []Value{"integer"}
		case float64:
			return []Value{"float"}
		}
		return []Value{nil}
	})

	rng := rand.New(rand.NewSource(1))
	add("random", func(_ *VM, args []Value) []Value {
		switch len(args) {
		case 0:
			return []Value{rng.Float64()}
		case 1:
			m := IntArg("math.random", 1, args[:1])
			if m == 0 {
				// Lua 5.4: math.random(0) returns a value with all bits random.
				return []Value{int64(rng.Uint64())}
			}
			if m < 0 {
				panic(LuaError("bad argument #1 to 'random' (interval is empty)"))
			}
			return []Value{int64(rng.Int63n(m)) + 1}
		default:
			lo := IntArg("math.random", 1, args)
			hi := IntArg("math.random", 2, args)
			if hi < lo {
				panic(LuaError("bad argument #2 to 'random' (interval is empty)"))
			}
			// Compute the span as unsigned to avoid int64 overflow on wide
			// intervals (e.g. mininteger..maxinteger). span == ^0 means the
			// full 2^64 range: every value is equally likely.
			span := uint64(hi) - uint64(lo)
			if span == ^uint64(0) {
				return []Value{int64(rng.Uint64())}
			}
			return []Value{lo + int64(rng.Uint64()%(span+1))}
		}
	})
	add("randomseed", func(_ *VM, args []Value) []Value {
		if len(args) >= 1 {
			seed := IntArg("math.randomseed", 1, args)
			rng = rand.New(rand.NewSource(seed))
		} else {
			rng = rand.New(rand.NewSource(1))
		}
		return nil
	})
	return t
}

// mathLess wraps the VM-level less() so math.max / math.min can use it on
// numeric pairs without a *VM (no metamethods are honoured here).
func mathLess(a, b Value) bool {
	switch x := a.(type) {
	case int64:
		switch y := b.(type) {
		case int64:
			return x < y
		case float64:
			return float64(x) < y
		}
	case float64:
		switch y := b.(type) {
		case int64:
			return x < float64(y)
		case float64:
			return x < y
		}
	}
	panic(Errorf("bad argument to 'math.max/min' (number expected, got %s)", TypeName(b)))
}

func floatToFloat1(name string, fn func(float64) float64) func(*VM, []Value) []Value {
	return func(_ *VM, args []Value) []Value {
		x := FloatArg(name, 1, args)
		return []Value{fn(x)}
	}
}

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
				panic(Errorf("bad argument #%d to 'char' (value out of range)", i+1))
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
			panic(Errorf("bad argument #3 to 'gsub' (string/table/function expected)"))
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
			panic(Errorf("bad argument #%d to 'format' (no value)", argIdx+2))
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
			panic(Errorf("invalid conversion '%s' to 'format'", format[i:]))
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
			panic(Errorf("invalid conversion '%%%c' to 'format'", verb))
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
		panic(Errorf("bad argument to 'format' (number has no integer representation)"))
	}
	if n, ok := ToInteger(v); ok {
		return n
	}
	panic(Errorf("bad argument to 'format' (number expected, got %s)", TypeName(v)))
}

// fmtArgFloat coerces a format argument to a float for the %f/%g/%e family.
func fmtArgFloat(v Value) float64 {
	if f, ok := ToFloat(v); ok {
		return f
	}
	panic(Errorf("bad argument to 'format' (number expected, got %s)", TypeName(v)))
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
	panic(Errorf("bad argument to 'format' (value has no literal form)"))
}

// table

func buildTableLibrary() *Table {
	t := NewTable(0, 8)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "table." + name, Fn: fn})
	}

	add("insert", func(_ *VM, args []Value) []Value {
		tbl := TableArg("insert", 1, args)
		switch len(args) {
		case 1:
			panic(LuaError("bad argument to 'insert' (value expected)"))
		case 2:
			// Append at the end.
			tbl.Set(tbl.Len()+1, args[1])
		case 3:
			pos := IntArg("table.insert", 2, args)
			val := args[2]
			n := tbl.Len()
			// Lua 5.4: the position must be in [1, n+1].
			if pos < 1 || pos > n+1 {
				panic(LuaError("bad argument #2 to 'insert' (position out of bounds)"))
			}
			// Shift right to make room for the new element.
			for i := n; i >= pos; i-- {
				tbl.Set(i+1, tbl.Get(i))
			}
			tbl.Set(pos, val)
		default:
			panic(LuaError("wrong number of arguments to 'insert'"))
		}
		return nil
	})
	add("remove", func(_ *VM, args []Value) []Value {
		tbl := TableArg("remove", 1, args)
		n := tbl.Len()
		if n == 0 {
			return []Value{nil}
		}
		pos := OptInt("table.remove", 2, args, n)
		// Lua 5.4 validates pos unless it equals the default (n); an out-of-range
		// pos must error, not silently drive a multi-billion-iteration shift loop.
		if pos != n && (pos < 1 || pos > n+1) {
			panic(LuaError("bad argument #2 to 'remove' (position out of bounds)"))
		}
		removed := tbl.Get(pos)
		for i := pos; i < n; i++ {
			tbl.Set(i, tbl.Get(i+1))
		}
		if pos <= n {
			tbl.Set(n, nil)
		}
		return []Value{removed}
	})
	add("concat", func(_ *VM, args []Value) []Value {
		tbl := TableArg("concat", 1, args)
		sep := OptString("table.concat", 2, args, "")
		lo := OptInt("table.concat", 3, args, 1)
		hi := OptInt("table.concat", 4, args, tbl.Len())
		var b strings.Builder
		for i := lo; i <= hi; i++ {
			if i > lo {
				b.WriteString(sep)
			}
			el := tbl.Get(i)
			switch e := el.(type) {
			case string:
				b.WriteString(e)
			case int64, float64:
				b.WriteString(ToString(el))
			default:
				// Reference Lua errors here; silently rendering "nil" or
				// "table: 0x…" into the result masks caller bugs.
				panic(Errorf("invalid value (%s) at index %d in table for 'concat'", TypeName(el), i))
			}
		}
		return []Value{b.String()}
	})
	add("sort", func(v *VM, args []Value) []Value {
		tbl := TableArg("sort", 1, args)
		var less func(a, b Value) bool
		if len(args) >= 2 && args[1] != nil {
			cmp := args[1]
			if _, ok := cmp.(*Closure); !ok {
				if _, ok := cmp.(*GoFunc); !ok {
					panic(Errorf("bad argument #2 to 'sort' (function expected, got %s)", TypeName(cmp)))
				}
			}
			less = func(a, b Value) bool {
				rs := v.CallValue(cmp, []Value{a, b}, 1)
				return len(rs) > 0 && IsTruthy(rs[0])
			}
		} else {
			less = v.lessMM
		}
		n := tbl.Len()
		elems := make([]Value, n)
		for i := int64(0); i < n; i++ {
			elems[i] = tbl.Get(i + 1)
		}
		sort.SliceStable(elems, func(i, j int) bool { return less(elems[i], elems[j]) })
		for i, e := range elems {
			tbl.Set(int64(i)+1, e)
		}
		return nil
	})
	add("move", func(_ *VM, args []Value) []Value {
		a1 := TableArg("move", 1, args)
		f := IntArg("table.move", 2, args)
		e := IntArg("table.move", 3, args)
		d := IntArg("table.move", 4, args)
		a2 := a1
		if len(args) >= 5 && args[4] != nil {
			a2 = TableArg("move", 5, args)
		}
		if e >= f {
			// Same wide-span overflow guard as unpack: hi-lo+1 can wrap.
			if uint64(e)-uint64(f) >= 1<<24 {
				panic(LuaError("too many elements to move"))
			}
			if d > e || d <= f || a1 != a2 {
				for i := int64(0); i <= e-f; i++ {
					a2.Set(d+i, a1.Get(f+i))
				}
			} else {
				// Overlapping shift within one table: copy back-to-front
				// so sources aren't overwritten before they're read.
				for i := e - f; i >= 0; i-- {
					a2.Set(d+i, a1.Get(f+i))
				}
			}
		}
		return []Value{a2}
	})
	add("unpack", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return nil
		}
		tbl := TableArg("unpack", 1, args)
		lo := OptInt("table.unpack", 2, args, 1)
		hi := OptInt("table.unpack", 3, args, tbl.Len())
		if hi < lo {
			return nil
		}
		// The element count is hi-lo+1, but that expression overflows int64 for a
		// wide (lo,hi) span (e.g. mininteger..maxinteger), wrapping negative/small
		// and bypassing the guard while the loop counter itself wraps and never
		// terminates. Compute the span in uint64, which is exact here because
		// hi>=lo: uint64(hi)-uint64(lo) == the true non-negative difference.
		if uint64(hi)-uint64(lo) >= 1<<24 {
			panic(LuaError("too many results to unpack"))
		}
		out := make([]Value, 0, hi-lo+1)
		for i := lo; i <= hi; i++ {
			out = append(out, tbl.Get(i))
		}
		return out
	})
	add("pack", func(_ *VM, args []Value) []Value {
		tbl := NewTable(len(args), 1)
		for i, a := range args {
			tbl.Set(int64(i+1), a)
		}
		tbl.Set("n", int64(len(args)))
		return []Value{tbl}
	})
	return t
}

// io  (minimal — write, read("l"))

func buildIOLibrary() *Table {
	t := NewTable(0, 4)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "io." + name, Fn: fn})
	}

	stdinReader := bufio.NewReader(os.Stdin)
	add("write", func(v *VM, args []Value) []Value {
		for _, a := range args {
			fmt.Fprint(os.Stdout, ToStringMM(v, a))
		}
		return nil
	})
	add("read", func(_ *VM, args []Value) []Value {
		fmtArg := "l"
		if len(args) >= 1 {
			if s, ok := args[0].(string); ok {
				fmtArg = strings.TrimPrefix(s, "*")
			}
		}
		switch fmtArg {
		case "l", "L":
			line, err := stdinReader.ReadString('\n')
			if err != nil && line == "" {
				return []Value{nil}
			}
			if fmtArg == "l" {
				line = strings.TrimRight(line, "\r\n")
			}
			return []Value{line}
		default:
			panic(Errorf("io.read format %q not supported in this VM", fmtArg))
		}
	})
	return t
}

// Argument-validation helpers (NumArg, FloatArg, IntArg, StringArg,
// describeBadArg, plus TableArg/ClosureArg/etc.) live in stdlib_args.go.
