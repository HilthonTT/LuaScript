package compiler_test

import (
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
	"github.com/hilthontt/luascript/v3/internal/vm"
)

// Codegen caches "this chain has no ?. link" while descending a chain; a
// plain chain compiled first must not hide an optional link in a later one
// that shares its shape.
func TestOptionalLinkDetectedAfterPlainChain(t *testing.T) {
	src := `
local t = { a = {} }
local plain = t.a.b
local n = nil
assert(n?.a.b == nil)
assert(t.a?.b == nil)
assert(t.a.b?.c == nil)
assert(not pcall(function() return n.a.b end))
`
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := vm.New().Run(chunks[0]); err != nil {
		t.Fatalf("run: %v", err)
	}
}
