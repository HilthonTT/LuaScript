package bytecode

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func generateFromSource(t *testing.T, src string) *InstructionSet {
	t.Helper()
	p := parser.New(lexer.New(src))
	prog, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse error: %s", err.Message)
	}
	g := NewGenerator()
	g.InitTopLevelScope(prog)
	stmts := prog.Block.Statements
	if prog.Block.Return != nil {
		stmts = append(stmts, prog.Block.Return)
	}
	return g.GenerateInstructions(stmts)[0]
}

func assertSetsEqual(t *testing.T, path string, want, got *InstructionSet) {
	t.Helper()
	if want.Name() != got.Name() || want.Type() != got.Type() {
		t.Errorf("%s: name/type = (%q, %q), want (%q, %q)",
			path, got.Name(), got.Type(), want.Name(), want.Type())
	}
	if got.NumParams != want.NumParams || got.IsVararg != want.IsVararg || got.NumLocals != want.NumLocals {
		t.Errorf("%s: proto header (params=%d vararg=%v locals=%d), want (params=%d vararg=%v locals=%d)",
			path, got.NumParams, got.IsVararg, got.NumLocals,
			want.NumParams, want.IsVararg, want.NumLocals)
	}
	if len(got.Upvalues) != len(want.Upvalues) {
		t.Fatalf("%s: %d upvalues, want %d", path, len(got.Upvalues), len(want.Upvalues))
	}
	for i := range want.Upvalues {
		if got.Upvalues[i] != want.Upvalues[i] {
			t.Errorf("%s: upvalue %d = %+v, want %+v", path, i, got.Upvalues[i], want.Upvalues[i])
		}
	}
	if len(got.Instructions) != len(want.Instructions) {
		t.Fatalf("%s: %d instructions, want %d", path, len(got.Instructions), len(want.Instructions))
	}
	for i, w := range want.Instructions {
		gi := got.Instructions[i]
		if gi.Opcode != w.Opcode || gi.A != w.A || gi.B != w.B || gi.StrA != w.StrA {
			t.Errorf("%s[%d]: (%s A=%d B=%d StrA=%q), want (%s A=%d B=%d StrA=%q)",
				path, i, gi.ActionName(), gi.A, gi.B, gi.StrA,
				w.ActionName(), w.A, w.B, w.StrA)
		}
		if gi.BoxedAny != w.BoxedAny {
			t.Errorf("%s[%d]: BoxedAny = %#v, want %#v", path, i, gi.BoxedAny, w.BoxedAny)
		}
		if gi.SourceLine() != w.SourceLine() {
			t.Errorf("%s[%d]: sourceLine = %d, want %d", path, i, gi.SourceLine(), w.SourceLine())
		}
		if gi.Line() != i {
			t.Errorf("%s[%d]: line index = %d, want %d", path, i, gi.Line(), i)
		}
	}
	if len(got.Protos) != len(want.Protos) {
		t.Fatalf("%s: %d protos, want %d", path, len(got.Protos), len(want.Protos))
	}
	for i := range want.Protos {
		assertSetsEqual(t, path+".protos["+want.Protos[i].Name()+"]", want.Protos[i], got.Protos[i])
	}
}

func roundTrip(t *testing.T, main *InstructionSet) *InstructionSet {
	t.Helper()
	var buf bytes.Buffer
	if err := SerializeChunk(&buf, main); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	got, err := DeserializeChunk(&buf)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	return got
}

func TestSerializeRoundTrip(t *testing.T) {
	main := generateFromSource(t, `
		local counter = 0
		local function make(step)
			return function(...)
				counter = counter + step
				return counter, ...
			end
		end
		local inc = make(2)
		local t = { name = "x", 3.14, [1 + 1] = true }
		function t:describe(sep)
			return self.name .. sep
		end
		for i = 1, 10 do
			if i % 2 == 0 then
				counter = counter - 1
			else
				counter = counter + i
			end
		end
		while counter > 0 and t do
			counter = counter - 1
			break
		end
		return inc(t:describe("!"))
	`)
	got := roundTrip(t, main)
	assertSetsEqual(t, "main", main, got)
}

func TestSerializeRoundTripTryCatch(t *testing.T) {
	main := generateFromSource(t, `
		local function risky(n)
			if n > 2 then throw { code = n } end
			return n
		end
		local total = 0
		for i = 1, 5 do
			try
				total = total + risky(i)
				if i == 4 then break end
			catch e do
				total = total - 1
				continue
			end
		end
		try
			throw "outer"
		catch do
			total = total * 2
		end
		return total
	`)
	got := roundTrip(t, main)
	assertSetsEqual(t, "main", main, got)
}

func TestSerializeRejectsGarbage(t *testing.T) {
	if _, err := DeserializeChunk(strings.NewReader("not a chunk at all")); err == nil {
		t.Fatal("expected an error for garbage input")
	}
	if _, err := DeserializeChunk(strings.NewReader("")); err == nil {
		t.Fatal("expected an error for empty input")
	}
}

func TestSerializeRejectsTruncated(t *testing.T) {
	main := generateFromSource(t, `local x = 1 return x + 2`)
	var buf bytes.Buffer
	if err := SerializeChunk(&buf, main); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	full := buf.Bytes()
	for n := 0; n < len(full); n++ {
		if _, err := DeserializeChunk(bytes.NewReader(full[:n])); err == nil {
			t.Fatalf("truncated chunk of %d/%d bytes decoded without error", n, len(full))
		}
	}
}

func TestSerializeVersionMismatch(t *testing.T) {
	main := generateFromSource(t, `return 1`)
	var buf bytes.Buffer
	if err := SerializeChunk(&buf, main); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	raw := buf.Bytes()
	raw[len(serialMagic)]++
	if _, err := DeserializeChunk(bytes.NewReader(raw)); err == nil {
		t.Fatal("expected a version-mismatch error")
	}
}
