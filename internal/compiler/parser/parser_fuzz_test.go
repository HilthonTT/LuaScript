package parser

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/lexer"
)

func FuzzParser(f *testing.F) {
	seeds := []string{
		"",
		"local x = 1",
		"local a, b, c = 1, 2, 3",
		"local x <const> = 42",
		"local x <close> = io.open()",
		"function f() end",
		"function f(a, b, ...) return a + b end",
		"function t.a.b:m() end",
		"if a then x = 1 elseif b then x = 2 else x = 3 end",
		"while true do break end",
		"repeat x = x + 1 until x > 10",
		"for i = 1, 10, 2 do end",
		"for k, v in pairs(t) do end",
		"local t = { 1, 2, x = 3, [4] = 5; 6 }",
		"return 1, 2, 3",
		"::start:: goto start",
		"a.b[1]:m(2).c = 3",
		"print \"hello\"",
		"f { 1, 2 }",
		"do local x = 1 end",
		"1 = 2",
		")",
		"if true then x = 1",
		"local",
		"a = a < b == c",
		"a = 2 ^ 3 ^ 2",
		"local x: number = 1",
		"function f(x: string): number return 0 end",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		p := New(lexer.New(input))
		prog, err := p.ParseProgram()
		if prog == nil && err == nil {
			t.Fatalf("both prog and err nil for %q", input)
		}
		if err == nil && prog != nil && prog.Block == nil {
			t.Fatalf("prog returned with nil Block for %q", input)
		}
	})
}
