package parser

// Tests for the language-surface additions: the contextual `continue`
// statement, Luau-style if expressions, default parameter values, and
// `<const>` / `<close>` attribute validation.

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/ast"
)

func TestParseContinueInLoops(t *testing.T) {
	srcs := []string{
		"for i = 1, 10 do continue end",
		"for k, v in pairs(t) do continue end",
		"while true do continue end",
		"repeat continue until done",
		"for i = 1, 10 do if i % 2 == 0 then continue end end",
	}
	for _, src := range srcs {
		prog := parse(t, src)
		found := false
		var walk func(b *ast.Block)
		var walkStmt func(s ast.Statement)
		walkStmt = func(s ast.Statement) {
			switch n := s.(type) {
			case *ast.ContinueStatement:
				found = true
			case *ast.NumericForStatement:
				walk(n.Body)
			case *ast.GenericForStatement:
				walk(n.Body)
			case *ast.WhileStatement:
				walk(n.Body)
			case *ast.RepeatStatement:
				walk(n.Body)
			case *ast.IfStatement:
				for _, c := range n.Clauses {
					walk(c.Body)
				}
			}
		}
		walk = func(b *ast.Block) {
			if b == nil {
				return
			}
			for _, s := range b.Statements {
				walkStmt(s)
			}
		}
		walk(prog.Block)
		if !found {
			t.Errorf("no ContinueStatement parsed from %q", src)
		}
	}
}

func TestParseContinueOutsideLoopFails(t *testing.T) {
	msg := parseError(t, "continue")
	if !strings.Contains(msg, "'continue' outside a loop") {
		t.Errorf("unexpected error message: %s", msg)
	}
	// break-style scoping: continue must not escape a function boundary.
	msg = parseError(t, "for i = 1, 3 do local f = function() continue end end")
	if !strings.Contains(msg, "'continue' outside a loop") {
		t.Errorf("unexpected error message: %s", msg)
	}
}

func TestContinueRemainsUsableAsIdentifier(t *testing.T) {
	// Each of these uses `continue` as an ordinary name; none may parse as
	// the continue statement.
	srcs := []string{
		"local continue = 1",
		"continue = 1",
		"continue()",
		"continue.x = 2",
		"continue[1] = 2",
		"continue:m()",
		"continue, x = 1, 2",
		"continue += 1",
		"local x = continue + 1",
	}
	for _, src := range srcs {
		prog := parse(t, src)
		for _, s := range prog.Block.Statements {
			if _, ok := s.(*ast.ContinueStatement); ok {
				t.Errorf("%q wrongly parsed as a continue statement", src)
			}
		}
	}
}

func TestParseIfExpression(t *testing.T) {
	stmt := parseExpect1(t, `local s = if x > 0 then "pos" elseif x < 0 then "neg" else "zero"`)
	ls, ok := stmt.(*ast.LocalStatement)
	if !ok {
		t.Fatalf("expected LocalStatement, got %T", stmt)
	}
	ie, ok := ls.Values[0].(*ast.IfExpression)
	if !ok {
		t.Fatalf("expected IfExpression value, got %T", ls.Values[0])
	}
	if len(ie.Clauses) != 2 {
		t.Fatalf("expected 2 clauses (if + elseif), got %d", len(ie.Clauses))
	}
	if ie.Else == nil {
		t.Fatal("else arm missing")
	}
}

func TestParseIfExpressionNesting(t *testing.T) {
	// If expressions in call args stop at `,` and nest.
	parse(t, "f(if a then 1 else 2, if b then 3 else if c then 4 else 5)")
	parse(t, "return if a then 1 else 2")
	parse(t, "local t = { if a then 1 else 2 }")
}

func TestParseIfExpressionRequiresElse(t *testing.T) {
	msg := parseError(t, "local x = if a then 1")
	if !strings.Contains(msg, "else") {
		t.Errorf("unexpected error message: %s", msg)
	}
}

func TestParseDefaultParams(t *testing.T) {
	stmt := parseExpect1(t, `local function f(a, b: number = 2, c: string = "x") end`)
	lf, ok := stmt.(*ast.LocalFunctionStatement)
	if !ok {
		t.Fatalf("expected LocalFunctionStatement, got %T", stmt)
	}
	ps := lf.Func.Params
	if len(ps) != 3 {
		t.Fatalf("expected 3 params, got %d", len(ps))
	}
	if ps[0].Default != nil {
		t.Error("param a should have no default")
	}
	if ps[1].Default == nil || ps[1].Type == nil {
		t.Error("param b should carry both a type and a default")
	}
	if ps[2].Default == nil {
		t.Error("param c should carry a default")
	}
}

func TestParseDefaultParamWithoutType(t *testing.T) {
	stmt := parseExpect1(t, "local function f(a = 1) end")
	lf := stmt.(*ast.LocalFunctionStatement)
	if lf.Func.Params[0].Default == nil {
		t.Fatal("expected default on untyped param")
	}
}

func TestParseAttribValidation(t *testing.T) {
	parse(t, "local x <const> = 1")
	parse(t, "local x <close> = nil")
	parse(t, "local x: number <const> = 1")
	msg := parseError(t, "local x <frozen> = 1")
	if !strings.Contains(msg, "unknown attribute 'frozen'") {
		t.Errorf("unexpected error message: %s", msg)
	}
}
