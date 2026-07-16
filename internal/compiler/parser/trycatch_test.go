package parser

// Tests for the `try ... catch [e] do ... end` statement and `throw`.

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

func parseTry(t *testing.T, src string) *ast.TryCatchStatement {
	t.Helper()
	stmt := parseExpect1(t, src)
	tc, ok := stmt.(*ast.TryCatchStatement)
	if !ok {
		t.Fatalf("expected *ast.TryCatchStatement, got %T for %q", stmt, src)
	}
	return tc
}

func TestParseTryCatchWithBinding(t *testing.T) {
	tc := parseTry(t, `try f() catch err do print(err) end`)
	if tc.CatchVar == nil {
		t.Fatal("expected a catch binding")
	}
	if tc.CatchVar.Name != "err" {
		t.Errorf("binding = %q, want %q", tc.CatchVar.Name, "err")
	}
	if len(tc.Try.Statements) != 1 {
		t.Errorf("try body has %d statements, want 1", len(tc.Try.Statements))
	}
	if len(tc.Catch.Statements) != 1 {
		t.Errorf("catch body has %d statements, want 1", len(tc.Catch.Statements))
	}
}

// TestParseTryCatchWithoutBinding — the binding is optional, the `do` is not.
func TestParseTryCatchWithoutBinding(t *testing.T) {
	tc := parseTry(t, `try f() catch do g() end`)
	if tc.CatchVar != nil {
		t.Errorf("expected no catch binding, got %q", tc.CatchVar.Name)
	}
	if len(tc.Catch.Statements) != 1 {
		t.Errorf("catch body has %d statements, want 1", len(tc.Catch.Statements))
	}
}

// TestParseTryCatchEmptyBodies — `catch` terminates the try block, so an empty
// protected body must not swallow the handler. The single `end` closes the
// whole statement; `catch ... do` opens no block of its own.
func TestParseTryCatchEmptyBodies(t *testing.T) {
	tc := parseTry(t, `try catch do end`)
	if len(tc.Try.Statements) != 0 {
		t.Errorf("try body has %d statements, want 0", len(tc.Try.Statements))
	}
	if len(tc.Catch.Statements) != 0 {
		t.Errorf("catch body has %d statements, want 0", len(tc.Catch.Statements))
	}
}

func TestParseTryCatchNested(t *testing.T) {
	tc := parseTry(t, `
try
    try
        f()
    catch inner do
        g()
    end
catch outer do
    h()
end`)
	if tc.CatchVar.Name != "outer" {
		t.Errorf("outer binding = %q, want %q", tc.CatchVar.Name, "outer")
	}
	if len(tc.Try.Statements) != 1 {
		t.Fatalf("outer try body has %d statements, want 1", len(tc.Try.Statements))
	}
	inner, ok := tc.Try.Statements[0].(*ast.TryCatchStatement)
	if !ok {
		t.Fatalf("expected a nested try, got %T", tc.Try.Statements[0])
	}
	if inner.CatchVar.Name != "inner" {
		t.Errorf("inner binding = %q, want %q", inner.CatchVar.Name, "inner")
	}
}

// TestParseTryBodyAllowsReturn — `return` is legal as the last statement of the
// protected block, like any other Lua block.
func TestParseTryBodyAllowsReturn(t *testing.T) {
	src := `function f() try return 1 catch e do return 2 end end`
	prog := parse(t, src)
	if len(prog.Block.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Block.Statements))
	}
	fn, ok := prog.Block.Statements[0].(*ast.FunctionDeclaration)
	if !ok {
		t.Fatalf("expected a function declaration, got %T", prog.Block.Statements[0])
	}
	tc, ok := fn.Func.Body.Statements[0].(*ast.TryCatchStatement)
	if !ok {
		t.Fatalf("expected a try statement, got %T", fn.Func.Body.Statements[0])
	}
	if tc.Try.Return == nil {
		t.Error("try block's return was not recorded")
	}
	if tc.Catch.Return == nil {
		t.Error("catch block's return was not recorded")
	}
}

func TestParseThrow(t *testing.T) {
	stmt := parseExpect1(t, `throw "boom"`)
	ts, ok := stmt.(*ast.ThrowStatement)
	if !ok {
		t.Fatalf("expected *ast.ThrowStatement, got %T", stmt)
	}
	if lit, ok := ts.Value.(*ast.StringLiteral); !ok || lit.Value != "boom" {
		t.Errorf("thrown value = %#v, want the string \"boom\"", ts.Value)
	}
}

// TestParseThrowNonStringValues — any expression may be thrown.
func TestParseThrowNonStringValues(t *testing.T) {
	for _, src := range []string{
		`throw 42`,
		`throw { code = 1, msg = "x" }`,
		`throw setmetatable({}, MyError)`,
		`throw err`,
	} {
		stmt := parseExpect1(t, src)
		ts, ok := stmt.(*ast.ThrowStatement)
		if !ok {
			t.Fatalf("expected *ast.ThrowStatement, got %T for %q", stmt, src)
		}
		if ts.Value == nil {
			t.Errorf("thrown value is nil for %q", src)
		}
	}
}

func TestTryCatchStatementString(t *testing.T) {
	tc := parseTry(t, `try f() catch e do g() end`)
	got := tc.String()
	for _, want := range []string{"try", "catch e", "do", "end"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to contain %q", got, want)
		}
	}
}

func TestParseTryCatchErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"no catch", `try f() end`, "missing 'catch'"},
		{"catch without do", `try f() catch e print(e) end`, "expected 'do' after 'catch'"},
		{"catch without do or binding", `try f() catch print(e) end`, "expected 'do' after 'catch'"},
		{"missing end", `try f() catch e do g()`, "missing 'end' to close 'try'"},
		{"try at eof", `try`, "missing 'catch'"},
		// `catch` is a block terminator, so a stray one ends the chunk rather
		// than reaching parseStatement — the same way a stray `end` behaves.
		{"stray catch", `catch e do end`, "unexpected 'catch' after end of chunk"},
		{"catch with no try", `do catch e do end end`, "missing 'end' to close 'do'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := parseError(t, tc.src)
			if !strings.Contains(msg, tc.want) {
				t.Errorf("error = %q, want it to contain %q", msg, tc.want)
			}
		})
	}
}

// TestParseTryCatchReportsOpeningLine — the "missing end" diagnostic should
// point the reader back at the `try` that opened the block.
func TestParseTryCatchReportsOpeningLine(t *testing.T) {
	msg := parseError(t, "local x = 1\n\ntry\n  f()\ncatch e do\n  g()\n")
	if !strings.Contains(msg, "line 3") {
		t.Errorf("error = %q, want it to cite line 3 (the `try`)", msg)
	}
}
