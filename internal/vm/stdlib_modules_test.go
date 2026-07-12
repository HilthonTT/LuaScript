package vm

import (
	"math"
	"testing"
)

// math

func TestMathConstants(t *testing.T) {
	v := run(t, `
		p = math.pi
		h = math.huge
		mi = math.mininteger
		ma = math.maxinteger
	`)
	if got, _ := global(t, v, "p").(float64); got < 3.14 || got > 3.15 {
		t.Errorf("math.pi = %v, want ~3.14159", got)
	}
	if got, _ := global(t, v, "h").(float64); !math.IsInf(got, 1) {
		t.Errorf("math.huge = %v, want +Inf", got)
	}
	assertGlobalEqual(t, v, "ma", int64(math.MaxInt64))
	assertGlobalEqual(t, v, "mi", int64(math.MinInt64))
}

func TestMathAbsAndCeilFloor(t *testing.T) {
	v := run(t, `
		a = math.abs(-7)
		b = math.abs(-3.5)
		c = math.ceil(2.3)
		d = math.floor(2.7)
	`)
	assertGlobalEqual(t, v, "a", int64(7))
	if got, _ := global(t, v, "b").(float64); got != 3.5 {
		t.Errorf("math.abs(-3.5) = %v, want 3.5", got)
	}
	assertGlobalEqual(t, v, "c", int64(3))
	assertGlobalEqual(t, v, "d", int64(2))
}

func TestMathSqrtAndExp(t *testing.T) {
	v := run(t, `
		a = math.sqrt(16)
		b = math.exp(0)
	`)
	if got, _ := global(t, v, "a").(float64); got != 4.0 {
		t.Errorf("math.sqrt(16) = %v, want 4", got)
	}
	if got, _ := global(t, v, "b").(float64); got != 1.0 {
		t.Errorf("math.exp(0) = %v, want 1", got)
	}
}

func TestMathMaxMin(t *testing.T) {
	v := run(t, `
		a = math.max(1, 5, 3, 2)
		b = math.min(1, 5, 3, 2)
	`)
	assertGlobalEqual(t, v, "a", int64(5))
	assertGlobalEqual(t, v, "b", int64(1))
}

func TestMathType(t *testing.T) {
	v := run(t, `
		a = math.type(1)
		b = math.type(1.0)
		c = math.type("x")
	`)
	assertGlobalEqual(t, v, "a", "integer")
	assertGlobalEqual(t, v, "b", "float")
	assertGlobalEqual(t, v, "c", nil)
}

func TestMathRandomDeterministicWithSeed(t *testing.T) {
	v := run(t, `
		math.randomseed(42)
		a = math.random()
		math.randomseed(42)
		b = math.random()
	`)
	// Same seed → same first draw.
	if !Equal(global(t, v, "a"), global(t, v, "b")) {
		t.Errorf("a = %v, b = %v, expected equal under same seed", global(t, v, "a"), global(t, v, "b"))
	}
}

// string

func TestStringBasicOps(t *testing.T) {
	v := run(t, `
		a = string.upper("hi")
		b = string.lower("HEY")
		c = string.len("hello")
		d = string.reverse("abc")
		e = string.rep("xy", 3)
		f = string.rep("xy", 3, "-")
	`)
	assertGlobalEqual(t, v, "a", "HI")
	assertGlobalEqual(t, v, "b", "hey")
	assertGlobalEqual(t, v, "c", int64(5))
	assertGlobalEqual(t, v, "d", "cba")
	assertGlobalEqual(t, v, "e", "xyxyxy")
	assertGlobalEqual(t, v, "f", "xy-xy-xy")
}

func TestStringSub(t *testing.T) {
	v := run(t, `
		a = string.sub("hello", 2, 4)
		b = string.sub("hello", -3)
		c = string.sub("hello", 2)
	`)
	assertGlobalEqual(t, v, "a", "ell")
	assertGlobalEqual(t, v, "b", "llo")
	assertGlobalEqual(t, v, "c", "ello")
}

func TestStringByteAndChar(t *testing.T) {
	v := run(t, `
		a = string.byte("A")
		s = string.char(72, 105)
	`)
	assertGlobalEqual(t, v, "a", int64(65))
	assertGlobalEqual(t, v, "s", "Hi")
}

func TestStringFindPlainSubstring(t *testing.T) {
	v := run(t, `
		s, e = string.find("hello world", "world")
		miss = string.find("hello", "xyz")
	`)
	assertGlobalEqual(t, v, "s", int64(7))
	assertGlobalEqual(t, v, "e", int64(11))
	assertGlobalEqual(t, v, "miss", nil)
}

func TestStringFormat(t *testing.T) {
	v := run(t, `
		a = string.format("%d-%s", 7, "ok")
		b = string.format("[%5d]", 42)
	`)
	assertGlobalEqual(t, v, "a", "7-ok")
	assertGlobalEqual(t, v, "b", "[   42]")
}

// String method-syntax: relies on the string metatable wired by the stdlib.
func TestStringMethodSyntax(t *testing.T) {
	v := run(t, `
		a = ("hi"):upper()
		b = ("HEY"):lower()
		c = ("hello"):len()
	`)
	assertGlobalEqual(t, v, "a", "HI")
	assertGlobalEqual(t, v, "b", "hey")
	assertGlobalEqual(t, v, "c", int64(5))
}

// table

func TestTableInsertAppendsAtEnd(t *testing.T) {
	v := run(t, `
		t = {1, 2}
		table.insert(t, 3)
		a, b, c = t[1], t[2], t[3]
		n = #t
	`)
	assertGlobalEqual(t, v, "a", int64(1))
	assertGlobalEqual(t, v, "b", int64(2))
	assertGlobalEqual(t, v, "c", int64(3))
	assertGlobalEqual(t, v, "n", int64(3))
}

func TestTableInsertAtPosition(t *testing.T) {
	v := run(t, `
		t = {1, 3}
		table.insert(t, 2, 99)
		a, b, c = t[1], t[2], t[3]
	`)
	assertGlobalEqual(t, v, "a", int64(1))
	assertGlobalEqual(t, v, "b", int64(99))
	assertGlobalEqual(t, v, "c", int64(3))
}

func TestTableRemoveLastByDefault(t *testing.T) {
	v := run(t, `
		t = {10, 20, 30}
		x = table.remove(t)
		n = #t
	`)
	assertGlobalEqual(t, v, "x", int64(30))
	assertGlobalEqual(t, v, "n", int64(2))
}

func TestTableConcat(t *testing.T) {
	v := run(t, `
		s = table.concat({"a", "b", "c"}, "-")
	`)
	assertGlobalEqual(t, v, "s", "a-b-c")
}

func TestTableUnpackAndPack(t *testing.T) {
	v := run(t, `
		p = table.pack(10, 20, 30)
		n = p.n
		a = p[1]
		b = p[2]
		c = p[3]
	`)
	assertGlobalEqual(t, v, "n", int64(3))
	assertGlobalEqual(t, v, "a", int64(10))
	assertGlobalEqual(t, v, "b", int64(20))
	assertGlobalEqual(t, v, "c", int64(30))
}

// io  (skip io.read which would block on stdin in tests)

func TestIOWriteIsCallable(t *testing.T) {
	// We don't capture stdout in tests; just verify the call doesn't panic.
	run(t, `io.write("")`)
}
