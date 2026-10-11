package iox_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
	"github.com/hilthontt/luascript/v3/internal/native/stdlib/iox"
	"github.com/hilthontt/luascript/v3/internal/vm"
)

func runIO(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	iox.RegisterIOPreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func TestFileHandleClosesOnToBeClosedVariable(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "data.txt"))
	v := runIO(t, fmt.Sprintf(`
		local io = require("io")
		local path = %q
		do
			local f <close> = io.open(path, "w")
			f:write("hello")
			held = f
		end
		_, closedMsg = held:write("again")
		do
			local f <close> = io.open(path, "r")
			content = f:read("a")
			f:close() -- explicit close first: __close must not raise afterwards
		end
		reached = true
	`, path))
	if got := v.Globals.Get("content"); got != "hello" {
		t.Fatalf("content = %v, want hello", got)
	}
	if got := v.Globals.Get("closedMsg"); got != "file is closed" {
		t.Fatalf("write after scope exit = %v, want \"file is closed\"", got)
	}
	if v.Globals.Get("reached") != true {
		t.Fatal("script did not finish")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
