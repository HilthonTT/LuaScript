package regexp_test

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	nativeregexp "github.com/hilthontt/luascript/internal/native/stdlib/regexp"
	"github.com/hilthontt/luascript/internal/vm"
)

func runRe(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	nativeregexp.RegisterRegexpPreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func eq(t *testing.T, v *vm.VM, name string, want vm.Value) {
	t.Helper()
	if got := v.Globals.Get(name); !vm.Equal(got, want) {
		t.Errorf("%s = %v (%T), want %v (%T)", name, got, got, want, want)
	}
}

func TestFindReturnsPositions(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("w(or)ld")
		s, e, g = re:find("hello world")
		miss = re:find("nothing here")
	`)
	eq(t, v, "s", int64(7))
	eq(t, v, "e", int64(11))
	eq(t, v, "g", "or")
	eq(t, v, "miss", nil)
}

func TestFindHonoursInit(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("ab")
		first = re:find("ab__ab")
		second = re:find("ab__ab", 3)
		fromEnd = re:find("ab__ab", -3)
		past = re:find("ab", 99)
	`)
	eq(t, v, "first", int64(1))
	eq(t, v, "second", int64(5))
	eq(t, v, "fromEnd", int64(5))
	eq(t, v, "past", nil)
}

func TestCaptureDistinguishesUnmatchedGroup(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("(a)|(b)")
		whole, g1, g2 = re:capture("a")
	`)
	eq(t, v, "whole", "a")
	eq(t, v, "g1", "a")
	eq(t, v, "g2", nil)
}

func TestGroupsReturnsNamedCaptures(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("(?P<user>\\w+)@(?P<host>[\\w.]+)")
		local g = re:groups("ada@example.com")
		user = g.user
		host = g.host
		miss = re:groups("nope")
	`)
	eq(t, v, "user", "ada")
	eq(t, v, "host", "example.com")
	eq(t, v, "miss", nil)
}

func TestFindAllCaptures(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("(\\w+)=(\\d+)")
		local all = re:find_all_captures("a=1, b=22")
		n = #all
		k1, v1 = all[1][2], all[1][3]
		k2, v2 = all[2][2], all[2][3]
		whole = all[1][1]
	`)
	eq(t, v, "n", int64(2))
	eq(t, v, "whole", "a=1")
	eq(t, v, "k1", "a")
	eq(t, v, "v1", "1")
	eq(t, v, "k2", "b")
	eq(t, v, "v2", "22")
}

func TestReplaceFunc(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("%(\\w+)%")
		out = re:replace_func("hi %name% and %other%", function(whole, key)
			if key == "name" then return "ada" end
			return "?"
		end)
		none = re:replace_func("nothing", function() return "x" end)
	`)
	eq(t, v, "out", "hi ada and ?")
	eq(t, v, "none", "nothing")
}

func TestFindAllLimit(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		local re = regexp.compile("a")
		all = #re:find_all("aaaa")
		two = #re:find_all("aaaa", 2)
	`)
	eq(t, v, "all", int64(4))
	eq(t, v, "two", int64(2))
}

func TestIsValid(t *testing.T) {
	v := runRe(t, `
		local regexp = require("regexp")
		good = regexp.is_valid("\\d+")
		bad = regexp.is_valid("(unclosed")
	`)
	eq(t, v, "good", true)
	eq(t, v, "bad", false)
}
