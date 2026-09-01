package vm

import (
	"math"
	"strings"
	"testing"
)

func TestPacksizeFixedFormats(t *testing.T) {
	v := run(t, `
		a = string.packsize("i4i4")
		b = string.packsize("<i8d")
		c = string.packsize("i1xi1")
		d = string.packsize("i4 i4")
		e = string.packsize("bBhHlLjJT")
	`)
	assertGlobalEqual(t, v, "a", int64(8))
	assertGlobalEqual(t, v, "b", int64(16))
	assertGlobalEqual(t, v, "c", int64(3))
	assertGlobalEqual(t, v, "d", int64(8))
	assertGlobalEqual(t, v, "e", int64(1+1+2+2+8+8+8+8+8))
}

func TestPacksizeAlignment(t *testing.T) {
	v := run(t, `
		a = string.packsize("!4i1i4")
		b = string.packsize("!8i1Xi4i4")
		c = string.packsize("i1i4")
	`)
	assertGlobalEqual(t, v, "a", int64(8))
	assertGlobalEqual(t, v, "b", int64(8))
	assertGlobalEqual(t, v, "c", int64(5))
}

func TestPackEndianness(t *testing.T) {
	v := run(t, `
		local le = string.pack("<i4", 1)
		local be = string.pack(">i4", 1)
		l1, l4 = string.byte(le, 1), string.byte(le, 4)
		b1, b4 = string.byte(be, 1), string.byte(be, 4)
	`)
	assertGlobalEqual(t, v, "l1", int64(1))
	assertGlobalEqual(t, v, "l4", int64(0))
	assertGlobalEqual(t, v, "b1", int64(0))
	assertGlobalEqual(t, v, "b4", int64(1))
}

func TestPackUnpackRoundTrip(t *testing.T) {
	v := run(t, `
		i = string.unpack("<i4", string.pack("<i4", -12345))
		u = string.unpack("<I4", string.pack("<I4", 4000000000))
		d = string.unpack("d", string.pack("d", 3.5))
		f = string.unpack("f", string.pack("f", 0.5))
		z = string.unpack("z", string.pack("z", "hello"))
		s = string.unpack("<s4", string.pack("<s4", "hi"))
		j = string.unpack("<j", string.pack("<j", math.maxinteger))
		big = string.unpack("<i16", string.pack("<i16", -5))
		bigbe = string.unpack(">i16", string.pack(">i16", -5))
	`)
	assertGlobalEqual(t, v, "i", int64(-12345))
	assertGlobalEqual(t, v, "u", int64(4000000000))
	assertGlobalEqual(t, v, "d", 3.5)
	assertGlobalEqual(t, v, "f", 0.5)
	assertGlobalEqual(t, v, "z", "hello")
	assertGlobalEqual(t, v, "s", "hi")
	assertGlobalEqual(t, v, "j", int64(math.MaxInt64))
	assertGlobalEqual(t, v, "big", int64(-5))
	assertGlobalEqual(t, v, "bigbe", int64(-5))
}

func TestPackFixedWidthString(t *testing.T) {
	v := run(t, `
		s = string.pack("c5", "ab")
		n = #s
		got = string.unpack("c5", s)
	`)
	assertGlobalEqual(t, v, "n", int64(5))
	assertGlobalEqual(t, v, "got", "ab\x00\x00\x00")
}

func TestUnpackReturnsNextPosition(t *testing.T) {
	v := run(t, `
		local buf = string.pack("<i2i2", 7, 9)
		a, b, nxt = string.unpack("<i2i2", buf)
		second, nxt2 = string.unpack("<i2", buf, 3)
		neg = string.unpack("<i2", buf, -2)
	`)
	assertGlobalEqual(t, v, "a", int64(7))
	assertGlobalEqual(t, v, "b", int64(9))
	assertGlobalEqual(t, v, "nxt", int64(5))
	assertGlobalEqual(t, v, "second", int64(9))
	assertGlobalEqual(t, v, "nxt2", int64(5))
	assertGlobalEqual(t, v, "neg", int64(9))
}

func TestPackRejectsOverflow(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`string.pack("i1", 300)`, "integer overflow"},
		{`string.pack("i1", -200)`, "integer overflow"},
		{`string.pack("I2", -1)`, "unsigned overflow"},
		{`string.pack("c2", "abc")`, "longer than given size"},
		{`string.pack("z", "a\0b")`, "contains zeros"},
		{`string.pack("s1", string.rep("x", 300))`, "does not fit"},
	} {
		if msg := runErr(t, c.src); !strings.Contains(msg, c.want) {
			t.Errorf("%s: got %q, want it to mention %q", c.src, msg, c.want)
		}
	}
}

func TestPacksizeRejectsVariableWidth(t *testing.T) {
	for _, src := range []string{`string.packsize("z")`, `string.packsize("s4")`} {
		if msg := runErr(t, src); !strings.Contains(msg, "variable-size format") {
			t.Errorf("%s: got %q, want a variable-size error", src, msg)
		}
	}
}

func TestPackFormatErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`string.pack("Q", 1)`, "invalid format option"},
		{`string.packsize("!3i4")`, "alignment not power of 2"},
		{`string.pack("c", "x")`, "missing size for format option 'c'"},
		{`string.pack("i20", 1)`, "out of limits"},
		{`string.unpack("<i8", "ab")`, "data string too short"},
		{`string.unpack("z", "abc")`, "unfinished string"},
	} {
		if msg := runErr(t, c.src); !strings.Contains(msg, c.want) {
			t.Errorf("%s: got %q, want it to mention %q", c.src, msg, c.want)
		}
	}
}

func TestUnpackWideIntegerOutOfRange(t *testing.T) {
	msg := runErr(t, `
		local s = string.pack("<i8", -1) .. string.rep("\0", 8)
		string.unpack("<i16", s)
	`)
	if !strings.Contains(msg, "does not fit into Lua Integer") {
		t.Errorf("got %q, want a does-not-fit error", msg)
	}
}
