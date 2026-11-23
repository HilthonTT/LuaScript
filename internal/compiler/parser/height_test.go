package parser

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler/lexer"
)

// Left-associative chains grow the tree without recursing in the parser, so
// the nesting-depth limit never saw them; a few-MB `a+a+...` script then
// overflowed the Go stack (a fatal error, not a recoverable panic) in the
// recursive passes after parsing.
func TestOverlongChainsAreRejected(t *testing.T) {
	n := maxExprHeight + 10
	cases := map[string]string{
		"binary":        "local x = a" + strings.Repeat("+a", n),
		"index":         "local x = a" + strings.Repeat(".b", n),
		"call":          "local x = f" + strings.Repeat("()", n),
		"interpolation": "local x = `" + strings.Repeat("{a}", n) + "`",
		"nested":        "local x = " + strings.Repeat("(", 100) + "a" + strings.Repeat("+a)", 100) + strings.Repeat("+a", n-150),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(lexer.New(src)).ParseProgram()
			if err == nil || !strings.Contains(err.Message, "nests too deeply") {
				t.Fatalf("want a nesting error, got %v", err)
			}
		})
	}
}

func TestLongChainsUnderTheLimitParse(t *testing.T) {
	n := maxExprHeight / 2
	for _, src := range []string{
		"local x = a" + strings.Repeat("+a", n),
		"local x = a" + strings.Repeat(".b", n),
		"local x = `" + strings.Repeat("{a}", n/2) + "`",
	} {
		if _, err := New(lexer.New(src)).ParseProgram(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}
