package formatter

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/lexer"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// A statement beginning with `(` fuses onto the previous statement, because
// Lua ignores the line break between them: `print("a")` followed by
// `("b"):upper()` is one call, `print("a")("b"):upper()`. Source that parses
// at all must therefore have carried a `;`, and dropping it on the way out
// silently changes what the program does.
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
			// The strongest check: the result must still parse the same way,
			// i.e. as more than one statement.
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

// A long run of one operator must break at a single indentation level. Each
// binary node used to emit its own group and its own nest, so a broken chain
// came out as a staircase whose next formatting pass made different choices —
// formatting was not idempotent, and its output was not a canonical form.
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

	// Every continuation line of the chain sits at the same indent.
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

// Flattening must never absorb a parenthesised child: `a - (b - c)` and
// `a - b - c` are different numbers.
func TestFormatKeepsParenthesesInChains(t *testing.T) {
	out, err := Format("local x = 10 - (4 - 1)\n", Options{})
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	if !strings.Contains(out, "(4 - 1)") {
		t.Errorf("parentheses lost, changing the value:\n%s", out)
	}
}
