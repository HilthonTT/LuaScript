package vm

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func compileBench(b *testing.B, src string) *bytecode.InstructionSet {
	b.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		b.Fatalf("compile error: %v", err)
	}
	return chunks[0]
}

func runBench(b *testing.B, is *bytecode.InstructionSet) {
	b.Helper()
	v := New()
	if err := v.Run(is); err != nil {
		b.Fatalf("vm error: %v", err)
	}
}

func BenchmarkFibRecursive(b *testing.B) {
	is := compileBench(b, `
		local function fib(n)
			if n < 2 then return n end
			return fib(n-1) + fib(n-2)
		end
		r = fib(20)
	`)

	for b.Loop() {
		runBench(b, is)
	}
}

func BenchmarkTableInsertSequential(b *testing.B) {
	is := compileBench(b, `
		local t = {}
		for i = 1, 500 do t[i] = i end
		local s = 0
		for i = 1, 500 do s = s + t[i] end
		r = s
	`)

	for b.Loop() {
		runBench(b, is)
	}
}

func BenchmarkTableInsertHash(b *testing.B) {
	is := compileBench(b, `
		local t = {}
		t.a = 1; t.b = 2; t.c = 3; t.d = 4; t.e = 5
		t.f = 6; t.g = 7; t.h = 8; t.i = 9; t.j = 10
		local s = 0
		for k, v in pairs(t) do s = s + v end
		r = s
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBench(b, is)
	}
}

func BenchmarkTableLiteralChurn(b *testing.B) {
	is := compileBench(b, `
		local n = 0
		for i = 1, 200 do
			local t = {}
			n = n + 1
		end
		r = n
	`)

	for b.Loop() {
		runBench(b, is)
	}
}

func BenchmarkStringConcat(b *testing.B) {
	is := compileBench(b, `
		local s = ""
		for i = 1, 100 do s = s .. "x" end
		r = #s
	`)

	for b.Loop() {
		runBench(b, is)
	}
}

func BenchmarkGlobalAccess(b *testing.B) {
	is := compileBench(b, `
		g = 0
		for i = 1, 500 do g = g + 1 end
		r = g
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBench(b, is)
	}
}

func BenchmarkClosureCall(b *testing.B) {
	is := compileBench(b, `
		local function make()
			local n = 0
			return function()
				n = n + 1
				return n
			end
		end
		local c = make()
		local s = 0
		for i = 1, 200 do s = s + c() end
		r = s
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBench(b, is)
	}
}

func BenchmarkLocalArithmetic(b *testing.B) {
	is := compileBench(b, `
		local s = 0
		for i = 1, 1000 do s = s + i end
		r = s
	`)

	for b.Loop() {
		runBench(b, is)
	}
}
