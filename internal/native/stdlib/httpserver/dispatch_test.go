package httpserver

import (
	"net/http"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/vm"
)

func loadHandlers(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v := vm.New()
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("run: %v", err)
	}
	return v
}

func TestDispatchHandlerErrorStaysUsable(t *testing.T) {
	v := loadHandlers(t, `
		bad = function(req) error("boom") end
		good = function(req) return "ok" end
	`)
	bad := v.Globals.Get("bad")
	good := v.Globals.Get("good")
	req := vm.NewTable(0, 0)

	got := dispatch(v, bad, nil, req)
	if got.status != http.StatusInternalServerError {
		t.Fatalf("error handler status = %d, want %d", got.status, http.StatusInternalServerError)
	}

	got = dispatch(v, good, nil, req)
	if got.status != http.StatusOK {
		t.Fatalf("good handler status = %d, want 200", got.status)
	}
	if got.body != "ok" {
		t.Errorf("good handler body = %q, want %q", got.body, "ok")
	}
}

func TestDispatchNotFound(t *testing.T) {
	v := vm.New()
	got := dispatch(v, nil, nil, vm.NewTable(0, 0))
	if got.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got.status)
	}
}
