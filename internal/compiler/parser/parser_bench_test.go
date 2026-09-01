package parser

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/lexer"
)

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

func BenchmarkParseLargeFile(b *testing.B) {
	var sb strings.Builder
	for i := range 100 {
		sb.WriteString("local function f")
		sb.WriteString("_")
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
