package optimize

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func parseExpr(t *testing.T, src string) ast.Expression {
	t.Helper()
	p := parser.New(lexer.New("local _v = " + src))
	prog, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	Fold(prog)
	ls, ok := prog.Block.Statements[0].(*ast.LocalStatement)
	if !ok || len(ls.Values) != 1 {
		t.Fatalf("unexpected AST shape for %q", src)
	}
	return ls.Values[0]
}

func TestFoldInteger(t *testing.T) {
	cases := map[string]int64{
		"1 + 2":       3,
		"2 * 3 + 1":   7,
		"10 - 4":      6,
		"7 // 2":      3,
		"-7 // 2":     -4,
		"7 % 3":       1,
		"-7 % 3":      2,
		"3 < 5 and 9": 9,
		"-5":          -5,
		"- -5":        5,
		`#"hello"`:    5,
		"~0":          -1,
		"5 & 3":       1,
		"5 | 2":       7,
		"6 ~ 3":       5,
		"1 << 4":      16,
		"256 >> 4":    16,
		"1 << 64":     0,
		"(1 + 2) * 3": 9,
	}
	for src, want := range cases {
		got := parseExpr(t, src)
		lit, ok := got.(*ast.IntegerLiteral)
		if !ok {
			t.Errorf("%q: want IntegerLiteral, got %T", src, got)
			continue
		}
		if lit.Value != want {
			t.Errorf("%q: want %d, got %d", src, want, lit.Value)
		}
	}
}

func TestFoldFloat(t *testing.T) {
	cases := map[string]float64{
		"1 + 2.0":  3.0,
		"1 / 2":    0.5,
		"2 ^ 10":   1024.0,
		"7.0 // 2": 3.0,
		"5.5 % 2":  1.5,
		"-2.5":     -2.5,
	}
	for src, want := range cases {
		got := parseExpr(t, src)
		lit, ok := got.(*ast.FloatLiteral)
		if !ok {
			t.Errorf("%q: want FloatLiteral, got %T", src, got)
			continue
		}
		if lit.Value != want {
			t.Errorf("%q: want %v, got %v", src, want, lit.Value)
		}
	}
}

func TestFoldBool(t *testing.T) {
	cases := map[string]bool{
		"3 < 5":       true,
		"5 <= 5":      true,
		"9 > 100":     false,
		`"a" < "b"`:   true,
		"1 == 1":      true,
		"1 ~= 2":      true,
		`"x" == 1`:    false,
		"nil == nil":  true,
		"not true":    false,
		"not nil":     true,
		"not 0":       false,
		"not (1 + 1)": false,
	}
	for src, want := range cases {
		got := parseExpr(t, src)
		lit, ok := got.(*ast.BooleanLiteral)
		if !ok {
			t.Errorf("%q: want BooleanLiteral, got %T", src, got)
			continue
		}
		if lit.Value != want {
			t.Errorf("%q: want %v, got %v", src, want, lit.Value)
		}
	}
}

func TestFoldString(t *testing.T) {
	got := parseExpr(t, `"a" .. "b" .. "c"`)
	lit, ok := got.(*ast.StringLiteral)
	if !ok {
		t.Fatalf("want StringLiteral, got %T", got)
	}
	if lit.Value != "abc" {
		t.Errorf("want %q, got %q", "abc", lit.Value)
	}
}

func TestFoldLogicalShortCircuit(t *testing.T) {
	if got := parseExpr(t, "false and 5"); !isBool(got, false) {
		t.Errorf("`false and 5`: got %T", got)
	}
	if lit, ok := parseExpr(t, "nil or 7").(*ast.IntegerLiteral); !ok || lit.Value != 7 {
		t.Errorf("`nil or 7`: got %#v", parseExpr(t, "nil or 7"))
	}
	if lit, ok := parseExpr(t, "1 or 7").(*ast.IntegerLiteral); !ok || lit.Value != 1 {
		t.Errorf("`1 or 7`: got %#v", parseExpr(t, "1 or 7"))
	}
}

func TestNotFolded(t *testing.T) {
	unfolded := []string{
		"1 // 0",
		"1 % 0",
		"1 < 2.0",
		"1 == 2.0",
		"1 .. 2",
	}
	for _, src := range unfolded {
		got := parseExpr(t, src)
		if _, ok := got.(*ast.BinaryExpression); !ok {
			t.Errorf("%q: expected unchanged BinaryExpression, got %T", src, got)
		}
	}
}

func TestCallNotFolded(t *testing.T) {
	got := parseExpr(t, "foo(1 + 2)")
	call, ok := got.(*ast.CallExpression)
	if !ok {
		t.Fatalf("want CallExpression, got %T", got)
	}
	if lit, ok := call.Args[0].(*ast.IntegerLiteral); !ok || lit.Value != 3 {
		t.Errorf("call arg not folded: got %#v", call.Args[0])
	}
}

func TestFoldInsideFunctionBody(t *testing.T) {
	got := parseExpr(t, "function() return 2 * 21 end")
	fn, ok := got.(*ast.FunctionExpression)
	if !ok {
		t.Fatalf("want FunctionExpression, got %T", got)
	}
	if fn.Body == nil || fn.Body.Return == nil || len(fn.Body.Return.Values) != 1 {
		t.Fatalf("unexpected function body shape")
	}
	if lit, ok := fn.Body.Return.Values[0].(*ast.IntegerLiteral); !ok || lit.Value != 42 {
		t.Errorf("body expression not folded: got %#v", fn.Body.Return.Values[0])
	}
}

func isBool(e ast.Expression, want bool) bool {
	lit, ok := e.(*ast.BooleanLiteral)
	return ok && lit.Value == want
}
