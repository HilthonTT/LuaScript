package httpserver

import (
	"net/http"
	"testing"

	"github.com/hilthontt/luascript/compiler"
	"github.com/hilthontt/luascript/compiler/parser"
	"github.com/hilthontt/luascript/vm"
)

// loadHandlers compiles src on a fresh VM and returns it so the test can pull
// handler globals out by name.
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

// TestDispatchHandlerErrorStaysUsable is the regression guard for the fix that
// routes handler calls through vm.SafeCall: a handler that errors must yield a
// 500 AND leave the shared VM clean, so a subsequent good request still works.
func TestDispatchHandlerErrorStaysUsable(t *testing.T) {
	v := loadHandlers(t, `
		bad = function(req) error("boom") end
		good = function(req) return "ok" end
	`)
	bad := v.Globals.Get("bad")
	good := v.Globals.Get("good")
	req := vm.NewTable(0, 0)

	// First request: the handler errors → 500, body carries the message.
	got := dispatch(v, bad, nil, req)
	if got.status != http.StatusInternalServerError {
		t.Fatalf("error handler status = %d, want %d", got.status, http.StatusInternalServerError)
	}

	// Second request on the SAME VM: if the failed call had corrupted the
	// VM's stack/frames, this would panic or misbehave.
	got = dispatch(v, good, nil, req)
	if got.status != http.StatusOK {
		t.Fatalf("good handler status = %d, want 200", got.status)
	}
	if got.body != "ok" {
		t.Errorf("good handler body = %q, want %q", got.body, "ok")
	}
}

// TestDispatchNotFound covers the no-handler default.
func TestDispatchNotFound(t *testing.T) {
	v := vm.New()
	got := dispatch(v, nil, nil, vm.NewTable(0, 0))
	if got.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got.status)
	}
}
