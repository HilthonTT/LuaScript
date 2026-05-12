package vm

import (
	"testing"

	"github.com/hilthontt/sakura-lang/compiler"
	"github.com/hilthontt/sakura-lang/compiler/bytecode"
	"github.com/hilthontt/sakura-lang/compiler/parser"
)

// compileBench compiles src once for use across b.N iterations. Compile
// failures fail the benchmark; do not measure compile time here — these
// benches target the VM, not the front end.
func compileBench(b *testing.B, src string) *bytecode.InstructionSet {
	b.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		b.Fatalf("compile error: %v", err)
	}
	return chunks[0]
}

// runBench creates a fresh VM and runs `is` once. A new VM per iteration
// keeps state isolated (no cross-iteration pollution of globals/stack) and
// includes VM construction in the measurement — which is what we want for
// "run a small program end-to-end" benchmarks.
func runBench(b *testing.B, is *bytecode.InstructionSet) {
	b.Helper()
	v := New()
	if err := v.Run(is); err != nil {
		b.Fatalf("vm error: %v", err)
	}
}

// BenchmarkFibRecursive — call-heavy. Recursive fib(20) exercises function
// call setup/teardown, the doCall arg-copy path, and stack growth.
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

// BenchmarkTableInsertSequential — table-heavy. 500 sequential integer
// inserts exercise the array-part promotion path in vm/table.go.
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

// BenchmarkTableInsertHash — hash-part-heavy. String keys force every
// store into the hash map; this measures map churn and the keys-slice
// bookkeeping.
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

// BenchmarkTableLiteralChurn — many ephemeral empty tables. Targets the
// NewTable opcode allocation cost (P-2).
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

// BenchmarkStringConcat — concat-heavy loop. Each iteration produces a
// new string via concatPair; tests garbage churn.
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

// BenchmarkGlobalAccess — tight GetGlobal/SetGlobal loop. Each iteration
// pays one map lookup + normalizeKey + interface assertion per access.
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

// BenchmarkClosureCall — closure with upvalues called repeatedly.
// Targets makeClosure allocation and upvalue access.
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

// BenchmarkLocalArithmetic — tight loop in locals only. Baseline number
// for "ideal" execution (no globals, no tables, no calls).
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
