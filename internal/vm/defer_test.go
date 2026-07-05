package vm

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// preamble defines a global `rec(s)` that appends to a global `order` string,
// the observation channel every defer test uses.
const deferPreamble = `
order = ""
function rec(s) order = order .. s .. ";" end
`

// TestDeferRunsAfterBodyLIFO covers the core contract: deferred calls run when
// the function returns, after the body, in last-in-first-out order, and the
// function's return value is unaffected.
func TestDeferRunsAfterBodyLIFO(t *testing.T) {
	v := run(t, deferPreamble+`
function f()
    defer rec("a")
    defer rec("b")
    rec("body")
    return 42
end
result = f()
`)
	assertGlobalEqual(t, v, "order", "body;b;a;")
	assertGlobalEqual(t, v, "result", int64(42))
}

// TestDeferRunsOnFallOffEnd confirms a function with no explicit return still
// runs its defers (the Leave path).
func TestDeferRunsOnFallOffEnd(t *testing.T) {
	v := run(t, deferPreamble+`
function f()
    defer rec("cleanup")
    rec("body")
end
f()
`)
	assertGlobalEqual(t, v, "order", "body;cleanup;")
}

// TestDeferCapturesByUpvalue documents the deliberate difference from Go: the
// deferred call observes the captured variable's value at exit time, not at the
// point the defer statement ran.
func TestDeferCapturesByUpvalue(t *testing.T) {
	v := run(t, deferPreamble+`
function f()
    local x = 1
    defer rec(tostring(x))
    x = 2
end
f()
`)
	assertGlobalEqual(t, v, "order", "2;")
}

// TestDeferRunsOnError is the cleanup-on-error case: when the body errors and a
// pcall catches it, the defers of the unwound frames still run.
func TestDeferRunsOnError(t *testing.T) {
	v := run(t, deferPreamble+`
function boom()
    defer rec("cleanup")
    rec("before")
    error("kaboom")
    rec("after") -- unreachable
end
ok, msg = pcall(boom)
`)
	assertGlobalEqual(t, v, "order", "before;cleanup;")
	assertGlobalEqual(t, v, "ok", false)
}

// TestDeferAcrossNestedFramesOnError confirms LIFO ordering holds across frames
// when an error unwinds several activations at once: the innermost frame's
// defers run before the outer frame's.
func TestDeferAcrossNestedFramesOnError(t *testing.T) {
	v := run(t, deferPreamble+`
function inner()
    defer rec("inner")
    error("x")
end
function outer()
    defer rec("outer")
    inner()
end
pcall(outer)
`)
	assertGlobalEqual(t, v, "order", "inner;outer;")
}

// TestDeferOnlyAcceptsCalls confirms the parser rejects non-call defer targets.
func TestDeferOnlyAcceptsCalls(t *testing.T) {
	_, err := compiler.CompileToInstructions(`function f() defer 1 + 1 end`, parser.NormalMode)
	if err == nil {
		t.Fatal("expected a parse error for `defer 1 + 1`")
	}
}

// TestDeferMethodCall confirms `defer obj:method()` works (method-call form).
func TestDeferMethodCall(t *testing.T) {
	v := run(t, deferPreamble+`
local obj = { tag = "m" }
function obj:close() rec(self.tag) end
function f()
    defer obj:close()
    rec("body")
end
f()
`)
	assertGlobalEqual(t, v, "order", "body;m;")
}
