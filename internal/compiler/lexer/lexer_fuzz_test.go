package lexer

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/token"
)

// FuzzLexer asserts that no input — however malformed — can cause the lexer
// to panic or to loop indefinitely. The lexer is panic-safe today (no
// `panic(` call sites in the package), so this is a regression-lock.
//
// Seed corpus mirrors the inputs used by lexer_test.go so the seeded run
// (which executes whenever `go test ./...` runs) exercises the same
// shapes the unit tests cover. Run `go test -fuzz=FuzzLexer
// ./compiler/lexer/` to mutate beyond the seeds.
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
		"@",        // illegal character path
		"\x00",     // NUL byte
		"\xff\xfe", // invalid UTF-8
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
		// Cap iterations to keep a malformed input from looping forever
		// without making progress. Real programs are nowhere near this.
		for range 100000 {
			tok := l.NextToken()
			if tok.Type == token.EOF {
				return
			}
		}
		t.Fatalf("lexer did not reach EOF after 100k tokens on %q", input)
	})
}
