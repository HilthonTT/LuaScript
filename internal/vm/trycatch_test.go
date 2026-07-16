package vm

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// preamble defines a global `rec(s)` that appends to a global `order` string,
// the observation channel most of these tests use to assert on control flow.
const tryPreamble = `
order = ""
function rec(s) order = order .. s .. ";" end
`

// TestTryCatchesThrow is the core contract: `throw` in the body transfers to
// the handler with the thrown value bound, and the rest of the body is skipped.
func TestTryCatchesThrow(t *testing.T) {
	v := run(t, tryPreamble+`
try
    rec("body")
    throw "boom"
    rec("unreachable")
catch e do
    rec("caught:" .. e)
end
rec("after")
`)
	assertGlobalEqual(t, v, "order", "body;caught:boom;after;")
}

// TestTryWithoutErrorSkipsCatch confirms the handler is dead code on the happy
// path — i.e. the EndTry + Jump the generator emits after the body.
func TestTryWithoutErrorSkipsCatch(t *testing.T) {
	v := run(t, tryPreamble+`
try
    rec("body")
catch e do
    rec("caught")
end
rec("after")
`)
	assertGlobalEqual(t, v, "order", "body;after;")
}

// TestTryCatchesRuntimeError covers faults the program never raised explicitly:
// a `try` catches anything `pcall` would.
func TestTryCatchesRuntimeError(t *testing.T) {
	v := run(t, `
local t = {}
try
    local _ = t.missing.field
    caught = "no"
catch e do
    caught = tostring(e)
end
`)
	got, _ := global(t, v, "caught").(string)
	if !strings.Contains(got, "index") {
		t.Errorf("caught = %q, want an index error", got)
	}
}

// TestTryCatchesErrorBuiltin — `throw` and `error` raise the same thing.
func TestTryCatchesErrorBuiltin(t *testing.T) {
	v := run(t, `
try
    error("via error")
catch e do
    caught = e
end
`)
	assertGlobalEqual(t, v, "caught", "via error")
}

// TestThrowPropagatesNonStringValues — any Lua value may be thrown, and it
// arrives at the handler verbatim rather than stringified.
func TestThrowPropagatesNonStringValues(t *testing.T) {
	v := run(t, `
try
    throw { code = 42 }
catch e do
    code = e.code
end
try
    throw 7
catch e do
    num = e
end
`)
	assertGlobalEqual(t, v, "code", int64(42))
	assertGlobalEqual(t, v, "num", int64(7))
}

// TestThrowIgnoresShadowedError pins that `throw` is a real raise rather than a
// lowering to a call to whatever `error` currently names.
func TestThrowIgnoresShadowedError(t *testing.T) {
	v := run(t, `
local error = function() end   -- shadow it: throw must not route through this
try
    throw "raised"
catch e do
    caught = e
end
`)
	assertGlobalEqual(t, v, "caught", "raised")
}

// TestCatchWithoutBinding — the binding is optional; the error value must still
// be consumed so the stack stays balanced.
func TestCatchWithoutBinding(t *testing.T) {
	v := run(t, tryPreamble+`
try
    throw "discarded"
catch do
    rec("caught")
end
rec("after")
`)
	assertGlobalEqual(t, v, "order", "caught;after;")
}

// TestTryCatchesErrorFromNestedCall confirms the unwind crosses call frames.
func TestTryCatchesErrorFromNestedCall(t *testing.T) {
	v := run(t, `
local function deep(n)
    if n == 0 then throw "from depth" end
    return deep(n - 1)
end
try
    deep(5)
catch e do
    caught = e
end
`)
	assertGlobalEqual(t, v, "caught", "from depth")
}

// TestReturnInsideTryReturnsFromEnclosingFunction is the payoff of compiling
// the body as a real protected region rather than wrapping it in a closure: a
// `return` acts on the enclosing function, not on a wrapper.
func TestReturnInsideTryReturnsFromEnclosingFunction(t *testing.T) {
	v := run(t, `
function f()
    try
        return "inner"
    catch e do
        return "handler"
    end
    return "fellthrough"
end
result = f()
`)
	assertGlobalEqual(t, v, "result", "inner")
}

// TestBreakInsideTryBreaksEnclosingLoop — likewise for `break`.
func TestBreakInsideTryBreaksEnclosingLoop(t *testing.T) {
	v := run(t, tryPreamble+`
for i = 1, 5 do
    try
        if i == 3 then break end
        rec(tostring(i))
    catch e do
        rec("caught")
    end
end
rec("after")
`)
	assertGlobalEqual(t, v, "order", "1;2;after;")
}

// TestContinueInsideTryContinuesEnclosingLoop — likewise for `continue`.
func TestContinueInsideTryContinuesEnclosingLoop(t *testing.T) {
	v := run(t, tryPreamble+`
for i = 1, 4 do
    try
        if i % 2 == 0 then continue end
        rec(tostring(i))
    catch e do
        rec("caught")
    end
end
`)
	assertGlobalEqual(t, v, "order", "1;3;")
}

// TestBreakOutOfTryPopsHandler is the regression guard for the EndTry that
// compileBreak emits: after breaking out of a protected region, a later error
// in the same frame must NOT land in the catch clause already jumped out of.
func TestBreakOutOfTryPopsHandler(t *testing.T) {
	v := run(t, `
local function f()
    for i = 1, 3 do
        try
            break
        catch e do
            leaked = "break handler ran"
        end
    end
    throw "propagated"
end
ok, err = pcall(f)
`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "err", "propagated")
	if leaked := global(t, v, "leaked"); leaked != nil {
		t.Errorf("stale handler fired after break: %v", leaked)
	}
}

// TestContinueOutOfTryPopsHandler — same guard for compileContinue.
func TestContinueOutOfTryPopsHandler(t *testing.T) {
	v := run(t, `
local function f()
    for i = 1, 2 do
        try
            continue
        catch e do
            leaked = "continue handler ran"
        end
    end
    throw "propagated"
end
ok, err = pcall(f)
`)
	assertGlobalEqual(t, v, "err", "propagated")
	if leaked := global(t, v, "leaked"); leaked != nil {
		t.Errorf("stale handler fired after continue: %v", leaked)
	}
}

// TestBreakOutOfNestedTryPopsBothHandlers exercises EndTry with a count > 1.
func TestBreakOutOfNestedTryPopsBothHandlers(t *testing.T) {
	v := run(t, `
local function f()
    for i = 1, 3 do
        try
            try
                break
            catch e do
                leaked = "inner handler ran"
            end
        catch e do
            leaked = "outer handler ran"
        end
    end
    throw "propagated"
end
ok, err = pcall(f)
`)
	assertGlobalEqual(t, v, "err", "propagated")
	if leaked := global(t, v, "leaked"); leaked != nil {
		t.Errorf("stale handler fired after nested break: %v", leaked)
	}
}

// TestNestedTryInnerCatchWins — the innermost open handler takes the error.
func TestNestedTryInnerCatchWins(t *testing.T) {
	v := run(t, tryPreamble+`
try
    try
        throw "x"
    catch e do
        rec("inner:" .. e)
    end
catch e do
    rec("outer:" .. e)
end
`)
	assertGlobalEqual(t, v, "order", "inner:x;")
}

// TestErrorInCatchPropagatesOutward pins that the handler is popped *before*
// the catch clause runs: an error raised inside catch must escape to the
// enclosing try rather than re-entering its own handler (which would loop).
func TestErrorInCatchPropagatesOutward(t *testing.T) {
	v := run(t, tryPreamble+`
try
    try
        throw "first"
    catch e do
        rec("inner:" .. e)
        throw "second"
    end
catch e do
    rec("outer:" .. e)
end
`)
	assertGlobalEqual(t, v, "order", "inner:first;outer:second;")
}

// TestUncaughtThrowReachesHost — a throw with no enclosing try surfaces as an
// ordinary runtime error.
func TestUncaughtThrowReachesHost(t *testing.T) {
	msg := runErr(t, `throw "escaped"`)
	if !strings.Contains(msg, "escaped") {
		t.Errorf("error = %q, want it to mention the thrown value", msg)
	}
}

// TestThrowAfterTryCompletesIsNotCaught confirms the handler installed by a
// finished try does not linger for the rest of the frame.
func TestThrowAfterTryCompletesIsNotCaught(t *testing.T) {
	v := run(t, `
local function f()
    try
        local _ = 1
    catch e do
        leaked = "handler ran"
    end
    throw "after"
end
ok, err = pcall(f)
`)
	assertGlobalEqual(t, v, "err", "after")
	if leaked := global(t, v, "leaked"); leaked != nil {
		t.Errorf("handler fired after its try completed: %v", leaked)
	}
}

// TestTryInsidePcall / TestPcallInsideTry cover both nesting orders of the two
// error-trapping mechanisms.
func TestTryInsidePcall(t *testing.T) {
	v := run(t, `
ok, err = pcall(function()
    try
        throw "inner"
    catch e do
        error("rethrown:" .. e)
    end
end)
`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "err", "rethrown:inner")
}

func TestPcallInsideTryDoesNotReachCatch(t *testing.T) {
	v := run(t, tryPreamble+`
try
    local ok, err = pcall(function() throw "handled by pcall" end)
    rec("pcall:" .. tostring(ok) .. ":" .. tostring(err))
catch e do
    rec("catch should not run")
end
`)
	assertGlobalEqual(t, v, "order", "pcall:false:handled by pcall;")
}

// TestDeferRunsWhenCatchUnwinds — a frame abandoned by the unwind still runs
// its deferred calls, and they run before the handler body.
func TestDeferRunsWhenCatchUnwinds(t *testing.T) {
	v := run(t, tryPreamble+`
local function inner()
    defer rec("cleanup")
    throw "boom"
end
try
    inner()
catch e do
    rec("caught:" .. e)
end
`)
	assertGlobalEqual(t, v, "order", "cleanup;caught:boom;")
}

// TestCatchBindingIsScopedToHandler — the binding is a normal local, invisible
// outside the handler and assignable inside it.
func TestCatchBindingIsScopedToHandler(t *testing.T) {
	v := run(t, `
e = "global e"
try
    throw "thrown"
catch e do
    inside = e
    e = "reassigned"
    afterAssign = e
end
outside = e
`)
	assertGlobalEqual(t, v, "inside", "thrown")
	assertGlobalEqual(t, v, "afterAssign", "reassigned")
	assertGlobalEqual(t, v, "outside", "global e")
}

// TestLocalsSurviveCatch confirms the unwind's stack truncation does not eat
// the enclosing frame's locals.
func TestLocalsSurviveCatch(t *testing.T) {
	v := run(t, `
local function f()
    local before = "kept"
    local n = 0
    try
        n = 1
        throw "x"
    catch e do
        return before .. ":" .. tostring(n)
    end
end
result = f()
`)
	assertGlobalEqual(t, v, "result", "kept:1")
}

// TestTryInLoopRecoversEachIteration — the handler is re-installed per pass, so
// a loop can absorb an error on every iteration.
func TestTryInLoopRecoversEachIteration(t *testing.T) {
	v := run(t, tryPreamble+`
for i = 1, 3 do
    try
        if i % 2 == 1 then throw "odd" .. i end
        rec("even" .. i)
    catch e do
        rec("caught:" .. e)
    end
end
`)
	assertGlobalEqual(t, v, "order", "caught:odd1;even2;caught:odd3;")
}

// TestTryCatchInsideCoroutine covers the per-frame handler design across a
// yield/resume state swap — including yielding from inside both the protected
// body and the handler.
func TestTryCatchInsideCoroutine(t *testing.T) {
	v := run(t, `
local co = coroutine.create(function()
    try
        coroutine.yield("y1")
        throw "in coro"
    catch e do
        coroutine.yield("caught:" .. e)
    end
    return "done"
end)
_, a = coroutine.resume(co)
_, b = coroutine.resume(co)
_, c = coroutine.resume(co)
`)
	assertGlobalEqual(t, v, "a", "y1")
	assertGlobalEqual(t, v, "b", "caught:in coro")
	assertGlobalEqual(t, v, "c", "done")
}

// TestTryCatchesErrorInVariadicCallArgs guards the callMarks truncation in
// dispatchToHandler: an error raised between MarkArgs and its matching Call
// leaves a pending mark that would otherwise corrupt the next variadic call.
func TestTryCatchesErrorInVariadicCallArgs(t *testing.T) {
	v := run(t, `
local function multi() return 1, 2 end
local function boom() throw "in args" end
try
    print("x", boom(), multi())
catch e do
    caught = e
end
-- A variadic call after the unwind must still find its own args base.
result = string.format("%s-%s", "a", "b")
after = select("#", multi())
`)
	assertGlobalEqual(t, v, "caught", "in args")
	assertGlobalEqual(t, v, "result", "a-b")
	assertGlobalEqual(t, v, "after", int64(2))
}

// TestTryCatchInREPLMode covers the REPL's separate compile path, which the
// verifier cannot drive interactively. Each input is its own chunk, and
// chunk-root locals are promoted to globals there — the catch binding must NOT
// be (it is scoped to its handler, and is bound by SetLocal rather than by
// compileLocal, so promotion never applies to it).
func TestTryCatchInREPLMode(t *testing.T) {
	v := New()
	for _, src := range []string{
		`local kept = "survives"`,
		`try throw { code = 7 } catch e do caught = e.code end`,
		`try throw "x" catch e do escaped = kept end
		 leaked = e`,
	} {
		chunks, err := compiler.CompileToInstructions(src, parser.REPLMode)
		if err != nil {
			t.Fatalf("REPL-mode compile error: %v\nsource:\n%s", err, src)
		}
		if err := v.Run(chunks[0]); err != nil {
			t.Fatalf("REPL-mode run error: %v\nsource:\n%s", err, src)
		}
	}
	assertGlobalEqual(t, v, "caught", int64(7))
	// The handler can see a promoted top-level local from an earlier chunk...
	assertGlobalEqual(t, v, "escaped", "survives")
	// ...but the binding itself does not leak out of the handler.
	if leaked := global(t, v, "leaked"); leaked != nil {
		t.Errorf("catch binding leaked to a global: %v", leaked)
	}
}

// TestDeepRecursionInsideTryIsCatchable — the VM's own stack-overflow guard
// raises an ordinary catchable error.
func TestDeepRecursionInsideTryIsCatchable(t *testing.T) {
	v := run(t, `
local function recurse(n) return recurse(n + 1) end
try
    recurse(1)
catch e do
    caught = tostring(e)
end
`)
	got, _ := global(t, v, "caught").(string)
	if !strings.Contains(got, "stack overflow") {
		t.Errorf("caught = %q, want a stack overflow error", got)
	}
}
