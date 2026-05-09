package vm

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"reflect"
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

// ---------------------------------------------------------------------------
// math
// ---------------------------------------------------------------------------

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
		if len(args) == 0 {
			panic(LuaError("bad argument #1 to 'max' (value expected)"))
		}
		best := args[0]
		for _, a := range args[1:] {
			if mathLess(best, a) {
				best = a
			}
		}
		return []Value{best}
	})
	add("min", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			panic(LuaError("bad argument #1 to 'min' (value expected)"))
		}
		best := args[0]
		for _, a := range args[1:] {
			if mathLess(a, best) {
				best = a
			}
		}
		return []Value{best}
	})
	add("fmod", func(_ *VM, args []Value) []Value {
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
			return []Value{int64(rng.Int63n(m)) + 1}
		default:
			lo := IntArg("math.random", 1, args[:1])
			hi := IntArg("math.random", 2, args[1:2])
			if hi < lo {
				panic(LuaError("bad argument to 'random' (interval is empty)"))
			}
			return []Value{int64(rng.Int63n(hi-lo+1)) + lo}
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

// ---------------------------------------------------------------------------
// string
// ---------------------------------------------------------------------------

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
		return []Value{strings.ToUpper(s)}
	})
	add("lower", func(_ *VM, args []Value) []Value {
		s := StringArg("string.lower", 1, args)
		return []Value{strings.ToLower(s)}
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
		if sep == "" {
			return []Value{strings.Repeat(s, int(n))}
		}
		// (s..sep) * (n-1) + s
		parts := make([]string, n)
		for i := range parts {
			parts[i] = s
		}
		return []Value{strings.Join(parts, sep)}
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
		idx := int64(1)
		if len(args) >= 2 {
			idx = IntArg("string.byte", 2, args)
		}
		if idx < 1 || idx > int64(len(s)) {
			return []Value{nil}
		}
		return []Value{int64(s[idx-1])}
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
		// Plain-substring only — Lua patterns are deliberately out of scope
		// for this VM. For pattern-style searches the caller should pass a
		// trailing `true` to force plain mode (we treat any call as plain).
		s := StringArg("string.find", 1, args)
		pat := StringArg("string.find", 2, args)
		init := int64(1)
		if len(args) >= 3 {
			init = IntArg("string.find", 3, args)
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
	add("format", func(_ *VM, args []Value) []Value {
		// Thin wrapper around Go's fmt.Sprintf. Lua's % directives are
		// largely a subset of Go's, so most simple format strings work
		// directly. Documented divergence from Lua's spec.
		if len(args) == 0 {
			panic(LuaError("bad argument #1 to 'format' (string expected)"))
		}
		fmtStr := StringArg("string.format", 1, args)
		fargs := make([]any, 0, len(args)-1)
		for _, a := range args[1:] {
			fargs = append(fargs, luaToFormatArg(a))
		}
		return []Value{fmt.Sprintf(fmtStr, fargs...)}
	})
	return t
}

func luaToFormatArg(v Value) any {
	switch x := v.(type) {
	case nil:
		return "nil"
	case bool:
		return x
	case int64:
		return x
	case float64:
		return x
	case string:
		return x
	}
	return ToString(v)
}

// ---------------------------------------------------------------------------
// table
// ---------------------------------------------------------------------------

func buildTableLibrary() *Table {
	t := NewTable(0, 8)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "table." + name, Fn: fn})
	}

	add("insert", func(_ *VM, args []Value) []Value {
		if len(args) < 2 {
			panic(LuaError("bad argument to 'insert' (table expected)"))
		}
		tbl, ok := args[0].(*Table)
		if !ok {
			panic(Errorf("bad argument #1 to 'insert' (table expected, got %s)", TypeName(args[0])))
		}
		switch len(args) {
		case 2:
			// Append at the end.
			tbl.Set(tbl.Len()+1, args[1])
		default:
			pos := IntArg("table.insert", 2, args)
			val := args[2]
			n := tbl.Len()
			// Shift right to make room for the new element.
			for i := n; i >= pos; i-- {
				tbl.Set(i+1, tbl.Get(i))
			}
			tbl.Set(pos, val)
		}
		return nil
	})
	add("remove", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			panic(LuaError("bad argument #1 to 'remove' (table expected)"))
		}
		tbl, ok := args[0].(*Table)
		if !ok {
			panic(Errorf("bad argument #1 to 'remove' (table expected, got %s)", TypeName(args[0])))
		}
		n := tbl.Len()
		if n == 0 {
			return []Value{nil}
		}
		pos := n
		if len(args) >= 2 {
			pos = IntArg("table.remove", 2, args)
		}
		removed := tbl.Get(pos)
		for i := pos; i < n; i++ {
			tbl.Set(i, tbl.Get(i+1))
		}
		tbl.Set(n, nil)
		return []Value{removed}
	})
	add("concat", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			panic(LuaError("bad argument #1 to 'concat' (table expected)"))
		}
		tbl, ok := args[0].(*Table)
		if !ok {
			panic(Errorf("bad argument #1 to 'concat' (table expected, got %s)", TypeName(args[0])))
		}
		sep := ""
		if len(args) >= 2 {
			sep = StringArg("table.concat", 2, args)
		}
		lo := int64(1)
		hi := tbl.Len()
		if len(args) >= 3 {
			lo = IntArg("table.concat", 3, args)
		}
		if len(args) >= 4 {
			hi = IntArg("table.concat", 4, args)
		}
		var b strings.Builder
		for i := lo; i <= hi; i++ {
			if i > lo {
				b.WriteString(sep)
			}
			b.WriteString(ToString(tbl.Get(i)))
		}
		return []Value{b.String()}
	})
	add("unpack", func(_ *VM, args []Value) []Value {
		if len(args) == 0 {
			return nil
		}
		tbl, ok := args[0].(*Table)
		if !ok {
			panic(Errorf("bad argument #1 to 'unpack' (table expected, got %s)", TypeName(args[0])))
		}
		lo := int64(1)
		hi := tbl.Len()
		if len(args) >= 2 {
			lo = IntArg("table.unpack", 2, args)
		}
		if len(args) >= 3 {
			hi = IntArg("table.unpack", 3, args)
		}
		var out []Value
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

// ---------------------------------------------------------------------------
// io  (minimal — write, read("l"))
// ---------------------------------------------------------------------------

func buildIOLibrary() *Table {
	t := NewTable(0, 4)
	add := func(name string, fn func(*VM, []Value) []Value) {
		t.Set(name, &GoFunc{Name: "io." + name, Fn: fn})
	}

	stdinReader := bufio.NewReader(os.Stdin)
	add("write", func(_ *VM, args []Value) []Value {
		for _, a := range args {
			fmt.Fprint(os.Stdout, ToString(a))
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

func NumArg(name string, n int, args []Value) (int64, float64, bool, bool) {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (value expected)", n, name))
	}
	i, f, isInt, ok := ToNumber(args[n-1])
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (number expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return i, f, isInt, ok
}

// describeBadArg renders the type of a bad argument for error
// messages. For runtime-tracked values it falls through to TypeName,
// matching Lua's `type()` strings. For values that crossed an FFI
// boundary as a raw Go primitive, it appends an actionable hint —
// the most common host-module bug is forgetting to cast int / uint /
// FileMode / rune to int64 (or float32 to float64) before storing
// them on a *Table, which leaves the runtime with an opaque value it
// can't coerce.
func describeBadArg(v Value) string {
	base := TypeName(v)
	switch v.(type) {
	case nil, bool, int64, float64, string, *Table, *Closure, *GoFunc, *Coroutine:
		return base
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Uintptr:
		return base + " — host stored a Go integer; cast to int64 before passing it to the runtime"
	case reflect.Float32:
		return base + " — host stored a Go float32; cast to float64 before passing it to the runtime"
	case reflect.String:
		return base + " — host stored a non-string named string type; convert to plain `string` before passing it to the runtime"
	}
	return base
}

func FloatArg(name string, n int, args []Value) float64 {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (number expected)", n, name))
	}
	x, ok := ToFloat(args[n-1])
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (number expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return x
}

func IntArg(name string, n int, args []Value) int64 {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (number expected)", n, name))
	}
	x, ok := ToInteger(args[n-1])
	if !ok {
		panic(Errorf("bad argument #%d to '%s' (number expected, got %s)", n, name, describeBadArg(args[n-1])))
	}
	return x
}

func StringArg(name string, n int, args []Value) string {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (string expected)", n, name))
	}
	if s, ok := args[n-1].(string); ok {
		return s
	}
	if i, f, isInt, ok := ToNumber(args[n-1]); ok {
		// Lua allows numbers where strings are expected via implicit coercion.
		if isInt {
			return formatInteger(i)
		}
		return formatFloat(f)
	}
	panic(Errorf("bad argument #%d to '%s' (string expected, got %s)", n, name, describeBadArg(args[n-1])))
}
