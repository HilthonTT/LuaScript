package vm

import (
	"strings"
	"testing"
)

const tbcPreamble = `
order = ""
function rec(s) order = order .. s .. ";" end
function res(n)
    return setmetatable({}, {__close = function(_, e)
        if e == nil then rec(n) else rec(n .. ":" .. tostring(e)) end
    end})
end
`

func TestCloseRunsAtBlockEndInReverseOrder(t *testing.T) {
	v := run(t, tbcPreamble+`
do
    local a <close> = res("a")
    local b <close> = res("b")
    rec("body")
end
rec("after")
`)
	assertGlobalEqual(t, v, "order", "body;b;a;after;")
}

func TestCloseRunsBeforeFunctionReturns(t *testing.T) {
	v := run(t, tbcPreamble+`
function f()
    local x <close> = res("x")
    return 42
end
result = f()
rec("after")
`)
	assertGlobalEqual(t, v, "order", "x;after;")
	assertGlobalEqual(t, v, "result", int64(42))
}

func TestCloseRunsOnBreakAndContinue(t *testing.T) {
	v := run(t, tbcPreamble+`
for i = 1, 3 do
    local c <close> = res("c" .. i)
    if i == 2 then break end
end
`)
	assertGlobalEqual(t, v, "order", "c1;c2;")

	v = run(t, tbcPreamble+`
for i = 1, 3 do
    local c <close> = res("c" .. i)
    if i == 2 then continue end
end
`)
	assertGlobalEqual(t, v, "order", "c1;c2;c3;")
}

func TestCloseReceivesErrorAndRunsBeforeHandler(t *testing.T) {
	v := run(t, tbcPreamble+`
ok, err = pcall(function()
    local z <close> = res("z")
    error("boom", 0)
end)
rec("handler")
`)
	assertGlobalEqual(t, v, "order", "z:boom;handler;")
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "err", "boom")
}

func TestCloseRunsWhenTryCatches(t *testing.T) {
	v := run(t, tbcPreamble+`
try
    local a <close> = res("a")
    error("bad", 0)
catch e do
    rec("caught")
end
`)
	assertGlobalEqual(t, v, "order", "a:bad;caught;")
}

func TestCloseIgnoresFalseAndNil(t *testing.T) {
	v := run(t, tbcPreamble+`
do
    local a <close> = false
    local b <close> = nil
    rec("body")
end
`)
	assertGlobalEqual(t, v, "order", "body;")
}

func TestCloseRejectsNonClosableValue(t *testing.T) {
	v := run(t, tbcPreamble+`
ok, err = pcall(function()
    local w <close> = {}
end)
`)
	assertGlobalEqual(t, v, "ok", false)
	if s, _ := global(t, v, "err").(string); !strings.Contains(s, "variable 'w' got a non-closable value") {
		t.Fatalf("unexpected error message: %v", global(t, v, "err"))
	}
}

func TestCloseRunsWhenCoroutineIsClosed(t *testing.T) {
	v := run(t, tbcPreamble+`
local co = coroutine.create(function()
    local k <close> = res("k")
    coroutine.yield(1)
    rec("unreachable")
end)
coroutine.resume(co)
coroutine.close(co)
`)
	assertGlobalEqual(t, v, "order", "k;")
}
