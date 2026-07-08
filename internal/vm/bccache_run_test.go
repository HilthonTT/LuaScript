package vm

// Executes a chunk loaded from the bytecode cache and checks it behaves
// identically to a fresh compile — the end-to-end guarantee the serializer's
// structural round-trip test can't give.

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler/bccache"
)

func TestCachedChunkRunsIdentically(t *testing.T) {
	t.Setenv("LUASCRIPT_CACHE_DIR", t.TempDir())
	t.Setenv("LUASCRIPT_NOCACHE", "")

	// Closures + upvalues + loops + continue + if-expr + defaults + strings:
	// a reasonable cross-section of codegen surface.
	src := `
		local function make(step = 1)
			local n = 0
			return function()
				n = n + step
				return n
			end
		end
		local inc = make(3)
		local acc = ""
		for i = 1, 6 do
			if i % 2 == 0 then continue end
			acc = acc .. (if i > 3 then "+" else "-") .. inc()
		end
		result = acc
	`

	runChunk := func() Value {
		t.Helper()
		main, err := bccache.CompileCached(src)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		v := New()
		if err := v.Run(main); err != nil {
			t.Fatalf("run: %v", err)
		}
		return v.Globals.Get("result")
	}

	fresh := runChunk()  // miss: compiles and stores
	cached := runChunk() // hit: deserialized from disk
	if !Equal(fresh, cached) {
		t.Fatalf("cached run produced %v, fresh run produced %v", cached, fresh)
	}
	if want := "-3-6+9"; !Equal(fresh, want) {
		t.Fatalf("result = %v, want %q", fresh, want)
	}
}
