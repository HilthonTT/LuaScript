package lexer

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/token"
)

func FuzzLexer(f *testing.F) {
	seeds := []string{
		"",
		"x",
		"local x = 1",
		"local a, b, c = 1, 2, 3",
		"function f() return 42 end",
		"if true then x = 1 else x = 2 end",
		"for i = 1, 10 do print(i) end",
		"-- a line comment\nx = 1",
		"--[[ multi\nline\ncomment ]]\nx = 1",
		`"a string"`,
		`"escape \n \t \\ \""`,
		`'single quoted'`,
		"[[long string]]",
		"[[multi\nline\nstring]]",
		"0 1 42 0xFF 0x0 3.14 .5 1e10 2.5E-3",
		"+ - * / // % ^ # & | ~ << >> == ~= < <= > >=",
		". .. ... : ::",
		"@",
		"\x00",
		"\xff\xfe",
		"[[unterminated",
		`"unterminated`,
		"--[==[ nested level ]==]",
		"goto foo ::foo::",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("lexer panic on %q: %v", input, r)
			}
		}()
		l := New(input)
		for range 100000 {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				return
			}
		}
		t.Fatalf("lexer did not reach EOF after 100k tokens on %q", input)
	})
}
