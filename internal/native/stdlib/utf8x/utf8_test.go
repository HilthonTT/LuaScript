package utf8x_test

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/native/stdlib/utf8x"
	"github.com/hilthontt/luascript/internal/vm"
)

func runUTF8(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	utf8x.RegisterUTF8Preload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func TestCharEncodesASCIIAndMultibyte(t *testing.T) {
	v := runUTF8(t, `
		local utf8 = require("utf8")
		a = utf8.char(72, 105)
		b = utf8.char(0x20AC)
		c = utf8.char(0x1F600)
	`)
	if got := v.Globals.Get("a"); !vm.Equal(got, "Hi") {
		t.Errorf("utf8.char(72,105) = %q, want \"Hi\"", got)
	}
	if got := v.Globals.Get("b"); !vm.Equal(got, "€") {
		t.Errorf("utf8.char(0x20AC) = %q, want the euro sign", got)
	}
	if got := v.Globals.Get("c"); !vm.Equal(got, "\U0001F600") {
		t.Errorf("utf8.char(0x1F600) = %q, want U+1F600", got)
	}
}

func TestCharEncodesSurrogatesVerbatim(t *testing.T) {
	v := runUTF8(t, `
		local utf8 = require("utf8")
		s = utf8.char(0xD800)
		n = #s
		b1 = string.byte(s, 1)
		b2 = string.byte(s, 2)
		b3 = string.byte(s, 3)
	`)
	if got := v.Globals.Get("n"); !vm.Equal(got, int64(3)) {
		t.Fatalf("#utf8.char(0xD800) = %v, want 3", got)
	}
	for i, want := range []int64{0xED, 0xA0, 0x80} {
		name := [...]string{"b1", "b2", "b3"}[i]
		if got := v.Globals.Get(name); !vm.Equal(got, want) {
			t.Errorf("byte %d of utf8.char(0xD800) = %v, want %#x (got U+FFFD substitution?)", i+1, got, want)
		}
	}
}

func TestCharEncodesAboveUnicodeRange(t *testing.T) {
	v := runUTF8(t, `
		local utf8 = require("utf8")
		s = utf8.char(0x7FFFFFFF)
		n = #s
	`)
	if got := v.Globals.Get("n"); !vm.Equal(got, int64(6)) {
		t.Errorf("#utf8.char(0x7FFFFFFF) = %v, want 6", got)
	}
}

func TestCharRejectsOutOfRange(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`
		local utf8 = require("utf8")
		utf8.char(0x80000000)
	`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	utf8x.RegisterUTF8Preload(v)
	if e := v.Run(chunks[0]); e == nil {
		t.Fatal("utf8.char(0x80000000) succeeded; want an out-of-range error")
	}
}

func TestLenAndCodepoint(t *testing.T) {
	v := runUTF8(t, `
		local utf8 = require("utf8")
		n = utf8.len("héllo")
		c = utf8.codepoint("héllo", 2)
	`)
	if got := v.Globals.Get("n"); !vm.Equal(got, int64(5)) {
		t.Errorf("utf8.len(\"héllo\") = %v, want 5", got)
	}
	if got := v.Globals.Get("c"); !vm.Equal(got, int64(0xE9)) {
		t.Errorf("utf8.codepoint = %v, want 0xE9", got)
	}
}
