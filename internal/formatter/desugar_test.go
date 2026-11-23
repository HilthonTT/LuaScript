package formatter

import (
	"strings"
	"testing"
	"time"
)

// The parser desugars compound assignment and string interpolation into bare
// BinaryExpressions; printed without precedence-aware parentheses they
// re-parsed into a different program (or a comment, for `- -x`).
func TestFormatPreservesMeaningOfDesugaredExpressions(t *testing.T) {
	cases := []struct{ src, want string }{
		{"x *= a + b", "x = x * (a + b)\n"},
		{"x -= a - b", "x = x - (a - b)\n"},
		{"local y = `a{b}`:upper()", "local y = (\"a\" .. tostring(b)):upper()\n"},
		{"local y = `a{b}`.n", "local y = (\"a\" .. tostring(b)).n\n"},
		{"local y = #`a{b}c`", "local y = #(\"a\" .. tostring(b) .. \"c\")\n"},
		{"local y = `a{b}` ^ 2", "local y = (\"a\" .. tostring(b)) ^ 2\n"},
		{"local y = `a{b}c`", "local y = \"a\" .. tostring(b) .. \"c\"\n"},
		{"local y = - -x", "local y = - -x\n"},
		{"local y = a - b - c", "local y = a - b - c\n"},
		{"local y = 2 ^ 3 ^ 4", "local y = 2 ^ 3 ^ 4\n"},
		{"local y = -x ^ 2", "local y = -x ^ 2\n"},
	}
	for _, c := range cases {
		got, err := Format(c.src, Options{})
		if err != nil {
			t.Fatalf("Format(%q): %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("Format(%q)\n got %q\nwant %q", c.src, got, c.want)
		}
		if again, err := Format(got, Options{}); err != nil || again != got {
			t.Errorf("Format(%q) not stable: %q, %v", got, again, err)
		}
	}
}

// strconv.Quote emitted Go's \uXXXX escape, which the lexer reads as a bare
// `u` followed by text, so formatting changed the string's contents.
func TestFormatQuotesStringsWithLuaEscapes(t *testing.T) {
	src := `local s = "\u{200B}\u{7F}\x01\xff\"\\` + "\t" + `é"`
	want := `local s = "\u{200B}\x7f\x01\xff\"\\\té"` + "\n"
	got, err := Format(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// A value-less `return` ending a match arm swallowed the next arm's pattern
// as its return value once the source's `;` was dropped.
func TestFormatKeepsSeparatorAfterBareReturnInMatchArm(t *testing.T) {
	src := "local function f(x)\nmatch x do\n1 -> return;\n0 -> return\nend\nend"
	got, err := Format(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Format(got, Options{}); err != nil {
		t.Fatalf("formatted output no longer parses: %v\n%s", err, got)
	}
}

// concat used to copy its nested concat's parts at every level, which made
// formatting a long call/index chain quadratic in time and memory.
func TestFormatLongChainIsLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("large input")
	}
	src := "local x = a" + strings.Repeat(".b", 45000) + strings.Repeat("()", 45000)
	start := time.Now()
	if _, err := Format(src, Options{}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Fatalf("formatting took %v", d)
	}
}
