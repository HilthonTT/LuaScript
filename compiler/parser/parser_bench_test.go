package parser

import (
	"strings"
	"testing"

	"github.com/hilthontt/sakura-lang/compiler/lexer"
)

// BenchmarkParseSmokeProgram — parses the same multi-feature program used
// by TestSmokeProgramParses. Establishes a baseline for parser throughput
// on a small real-world chunk.
func BenchmarkParseSmokeProgram(b *testing.B) {
	src := `
local function fib(n)
  if n < 2 then return n end
  return fib(n - 1) + fib(n - 2)
end

local t = { 1, 2, 3 }
for i, v in ipairs(t) do
  print(i, v, fib(v))
end

local s = "hello" .. " " .. "world"
local x <const> = 42
return fib(10)
`

	for b.Loop() {
		p := New(lexer.New(src))
		if _, err := p.ParseProgram(); err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}

// BenchmarkParseLargeFile — synthesizes ~500 lines of mixed statements.
// Exercises the parser's growth paths (stmt slice, AST node count).
func BenchmarkParseLargeFile(b *testing.B) {
	var sb strings.Builder
	for i := range 100 {
		sb.WriteString("local function f")
		sb.WriteString("_")
		// distinct names so the parser can't dedup anything
		for j := range 3 {
			sb.WriteByte(byte('a' + (i+j)%26))
		}
		sb.WriteString("(x, y, z)\n")
		sb.WriteString("  if x < y then return z + 1 end\n")
		sb.WriteString("  local t = { x, y, z, x + y, y * z }\n")
		sb.WriteString("  for i = 1, 10 do t[i] = t[i] * 2 end\n")
		sb.WriteString("  return t[1] + t[2] + t[3]\n")
		sb.WriteString("end\n\n")
	}
	src := sb.String()

	for b.Loop() {
		p := New(lexer.New(src))
		if _, err := p.ParseProgram(); err != nil {
			b.Fatalf("parse error: %v", err)
		}
	}
}
