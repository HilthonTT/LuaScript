package vm

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

const deferPreamble = `
order = ""
function rec(s) order = order .. s .. ";" end
`

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

func TestDeferOnlyAcceptsCalls(t *testing.T) {
	_, err := compiler.CompileToInstructions(`function f() defer 1 + 1 end`, parser.NormalMode)
	if err == nil {
		t.Fatal("expected a parse error for `defer 1 + 1`")
	}
}

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
