package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
)

// FuzzCompile drives the whole pipeline (lex → parse → constcheck → typecheck
// → fold → codegen) in every mode a script can select. A malformed script must
// produce an error, never a panic; the parser's own recover() masks panics as
// "panic during parse", so that message is treated as a crash too.
func FuzzCompile(f *testing.F) {
	seeds := []string{
		"",
		"local x = 1 + 2 * 3",
		"--!strict\nlocal function f(a: number): number return a // 0 end",
		"--!strict\ntype T<U> = { v: U, next: T<U>? }\nlocal t: T<number> = { v = 1 }",
		"enum Color { Red, Green(number) }\nlocal c = Color.Red\nmatch c { Red => print(1), _ => print(2) }",
		"struct P { x: number }\nimpl P { function len(self) return self.x end }",
		"interface I { x: number }",
		"local s = `a {1 + 2} b`",
		"local x = 1 |> tostring",
		"local t = {} t[1] += 2",
		"local a, b = ...",
		"local {x, y, ...rest} = t",
		"try error(1) catch e then print(e) end",
		"::l:: goto l",
		"for i = 1, 10 do continue end",
		"local x = math.mininteger // -1",
		"local x = 1 % 0",
		"local x = 0x7fffffffffffffff + 1",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	if entries, err := os.ReadDir(filepath.Join("..", "..", "examples")); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".lsc") {
				continue
			}
			if b, err := os.ReadFile(filepath.Join("..", "..", "examples", e.Name())); err == nil {
				f.Add(string(b))
			}
		}
	}

	f.Fuzz(func(t *testing.T, input string) {
		for _, src := range []string{input, "--!strict\n" + input, "--!nocheck\n" + input} {
			for _, mode := range []parser.Mode{parser.NormalMode, parser.REPLMode} {
				_, err := CompileToInstructions(src, mode)
				if err != nil && strings.Contains(err.Error(), "panic during parse") {
					t.Fatalf("parser panicked on %q: %v", src, err)
				}
			}
		}
	})
}
