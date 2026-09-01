package formatter

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func TestFormatKeepsSeparatorBeforeParenStatement(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"method call on a parenthesised literal", `print("a");
("b"):upper()
print("done")`},
		{"parenthesised call", `local f = print
print("a");
(f)("b")`},
		{"assignment to a parenthesised target", `local t = {}
print("a");
(t).x = 1`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Format(c.src, Options{})
			if err != nil {
				t.Fatalf("Format failed: %v", err)
			}
			if !strings.Contains(out, ";") {
				t.Errorf("separator dropped — formatted output changes meaning:\n%s", out)
			}
			mustParseAsTwoOrMoreStatements(t, out)
		})
	}
}

func mustParseAsTwoOrMoreStatements(t *testing.T, src string) {
	t.Helper()
	prog, err := parser.New(lexer.New(src)).ParseProgram()
	if err != nil {
		t.Fatalf("formatted output no longer parses: %s\n%s", err.Message, src)
	}
	if prog.Block == nil || len(prog.Block.Statements) < 2 {
		t.Errorf("formatted output collapsed into one statement:\n%s", src)
	}
}

func TestFormatFlattensOperatorChains(t *testing.T) {
	src := `local qty, price = 3, 4.5
print("plain      : " .. tostring(qty) .. " x " .. tostring(price) .. " = " .. tostring(qty * price))`

	once, err := Format(src, Options{})
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	twice, err := Format(once, Options{})
	if err != nil {
		t.Fatalf("second Format failed: %v", err)
	}
	if once != twice {
		t.Fatalf("chain formatting is not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}

	var indents []string
	for _, line := range strings.Split(once, "\n") {
		if trimmed := strings.TrimLeft(line, " "); strings.HasPrefix(trimmed, "..") {
			indents = append(indents, line[:len(line)-len(trimmed)])
		}
	}
	if len(indents) < 2 {
		t.Fatalf("expected the chain to break over several lines, got:\n%s", once)
	}
	for _, got := range indents[1:] {
		if got != indents[0] {
			t.Fatalf("chain operands are not aligned (staircase):\n%s", once)
		}
	}
}

func TestFormatKeepsParenthesesInChains(t *testing.T) {
	out, err := Format("local x = 10 - (4 - 1)\n", Options{})
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	if !strings.Contains(out, "(4 - 1)") {
		t.Errorf("parentheses lost, changing the value:\n%s", out)
	}
}
