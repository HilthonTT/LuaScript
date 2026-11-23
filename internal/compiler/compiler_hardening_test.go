package compiler

import (
	"strings"
	"testing"
	"time"

	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
)

// Long left-deep chains used to be quadratic: the checker re-refined the whole
// `or`/`and` chain at every level, and codegen re-walked the whole a.b.c...
// chain looking for `?.` at every suffix. An 80k-term chain took minutes.
func TestLongChainsCompileInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("large inputs")
	}
	const n = 80000
	for name, src := range map[string]string{
		"or":     "local a = nil\nlocal x = a" + strings.Repeat(" or a", n),
		"and":    "local a = 1\nlocal x = a" + strings.Repeat(" and a", n),
		"index":  "local a = {}\nlocal x = a" + strings.Repeat("[1]", n),
		"call":   "local f = print\nlocal x = f" + strings.Repeat("()", n),
		"method": "local a = {}\nlocal x = a" + strings.Repeat(":m()", n),
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			if _, err := CompileToInstructions(src, parser.NormalMode); err != nil {
				t.Fatalf("compile: %v", err)
			}
			if d := time.Since(start); d > 20*time.Second {
				t.Fatalf("compiling took %v", d)
			}
		})
	}
}
