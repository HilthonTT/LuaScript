package vm

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// runFailing compiles src, stamps it with `chunk`, runs it, and requires that
// it fails — returning the *RuntimeError so tests can inspect message and
// traceback separately.
func runFailing(t *testing.T, chunk, src string) *RuntimeError {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	chunks[0].SetSource(chunk)
	e := New().Run(chunks[0])
	if e == nil {
		t.Fatalf("expected a runtime error\nsource:\n%s", src)
	}
	re, ok := e.(*RuntimeError)
	if !ok {
		t.Fatalf("Run returned %T (%v), want *RuntimeError", e, e)
	}
	return re
}

// runNamed is `run` with a chunk name, for tests that assert on positions.
func runNamed(t *testing.T, chunk, src string) *VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	chunks[0].SetSource(chunk)
	v := New()
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

// wantLines asserts that got contains each want in order, so a test pins the
// shape of a traceback without pinning every character of it.
func wantLines(t *testing.T, got string, want ...string) {
	t.Helper()
	rest := got
	for _, w := range want {
		i := strings.Index(rest, w)
		if i < 0 {
			t.Fatalf("missing %q (in the expected order) in:\n%s", w, got)
		}
		rest = rest[i+len(w):]
	}
}

// A VM-raised runtime error carries the chunk name and line it was raised at,
// which before this existed only for error().
func TestRuntimeErrorCarriesPosition(t *testing.T) {
	re := runFailing(t, "demo.lsc", `
local function boom(t)
    return t.x
end
boom(nil)
`)
	if want := "demo.lsc:3: attempt to index a nil value"; re.Message() != want {
		t.Errorf("message = %q, want %q", re.Message(), want)
	}
}

// The traceback names every Lua frame between the raise and the chunk root,
// innermost first, with the line each frame was executing.
func TestTracebackWalksTheCallChain(t *testing.T) {
	re := runFailing(t, "demo.lsc", `
local function inner(t)
    return t.x
end
local function middle(t)
    return inner(t)
end
function Outer(t)
    return middle(t)
end
Outer(nil)
`)
	wantLines(t, re.Error(),
		"demo.lsc:3: attempt to index a nil value",
		"stack traceback:",
		"demo.lsc:3: in function 'inner'",
		"demo.lsc:6: in function 'middle'",
		"demo.lsc:9: in function 'Outer'",
		"demo.lsc:11: in main chunk",
	)
}

// error() reports the chunk it was called from rather than the hardcoded
// "script" the old implementation used.
func TestErrorBuiltinUsesChunkName(t *testing.T) {
	re := runFailing(t, "mod/thing.lsc", `
local function f()
    error("nope")
end
f()
`)
	if want := "mod/thing.lsc:3: nope"; re.Message() != want {
		t.Errorf("message = %q, want %q", re.Message(), want)
	}
}

// A chunk nobody stamped keeps reporting as "script", which is what an
// unnamed REPL or load() chunk has always done.
func TestUnstampedChunkKeepsDefaultName(t *testing.T) {
	re := runFailing(t, "", "local t = nil\nreturn t.x\n")
	if !strings.HasPrefix(re.Message(), "script:2:") {
		t.Errorf("message = %q, want a script:2: prefix", re.Message())
	}
}

// error(level) still selects the frame to blame: level 2 points at the
// caller, which is how argument-validation helpers report their caller.
func TestErrorLevelSelectsFrame(t *testing.T) {
	re := runFailing(t, "demo.lsc", `
local function check()
    error("bad input", 2)
end
local function caller()
    check()
end
caller()
`)
	if want := "demo.lsc:6: bad input"; re.Message() != want {
		t.Errorf("message = %q, want %q", re.Message(), want)
	}
}

// A message that is not a string is a Lua error *value*, and positioning
// must not touch it — pcall and catch hand it back as it was raised.
func TestNonStringErrorValueIsNotPositioned(t *testing.T) {
	v := run(t, `
ok, err = pcall(function() error({ code = 42 }) end)
`)
	assertGlobalEqual(t, v, "ok", false)
	tbl, isTable := global(t, v, "err").(*Table)
	if !isTable {
		t.Fatalf("err = %v (%T), want the raised table verbatim", global(t, v, "err"), global(t, v, "err"))
	}
	if got := tbl.Get("code"); !Equal(got, int64(42)) {
		t.Errorf("err.code = %v, want 42", got)
	}
}

// The position is stamped at the raise site, not at the boundary that caught
// it: a pcall several frames up still reports the innermost line.
func TestPcallReportsRaisePosition(t *testing.T) {
	v := runNamed(t, "demo.lsc", `
local function deep(t) return t.x end
local function mid(t) return deep(t) end
ok, err = pcall(mid, nil)
`)
	got, _ := global(t, v, "err").(string)
	if want := "demo.lsc:2: attempt to index a nil value"; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}

// Same rule for a `try` region: the catch binding sees where the error came
// from, not where it was handled.
func TestCatchReportsRaisePosition(t *testing.T) {
	v := runNamed(t, "demo.lsc", `
local function deep(t) return t.x end
try
    deep(nil)
catch e do
    caught = e
end
`)
	got, _ := global(t, v, "caught").(string)
	if want := "demo.lsc:2: attempt to index a nil value"; got != want {
		t.Errorf("caught = %q, want %q", got, want)
	}
}

// A coroutine dying on a runtime error reports the position inside the
// coroutine body — its frames must be read before the goroutine unwinds.
func TestCoroutineErrorReportsPosition(t *testing.T) {
	v := runNamed(t, "demo.lsc", `
local co = coroutine.create(function(t)
    return t.x
end)
ok, err = coroutine.resume(co, nil)
`)
	assertGlobalEqual(t, v, "ok", false)
	got, _ := global(t, v, "err").(string)
	if want := "demo.lsc:3: attempt to index a nil value"; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}

// Runaway recursion must not render one traceback line per frame.
func TestDeepRecursionTracebackIsElided(t *testing.T) {
	re := runFailing(t, "demo.lsc", `
local function recur(n)
    return recur(n + 1)
end
recur(1)
`)
	if !strings.Contains(re.Message(), "stack overflow") {
		t.Fatalf("message = %q, want a stack overflow", re.Message())
	}
	lines := strings.Count(re.Error(), "\n")
	if max := tracebackHead + tracebackTail + 2; lines > max {
		t.Errorf("traceback has %d lines, want at most %d", lines, max)
	}
	if !strings.Contains(re.Error(), "... (skipping ") {
		t.Errorf("expected an elision marker in:\n%s", re.Error())
	}
}

// A one-frame stack adds nothing the message's own position prefix did not
// already say, so it is not printed.
func TestSingleFrameErrorOmitsTraceback(t *testing.T) {
	re := runFailing(t, "demo.lsc", "local t = nil\nreturn t.x\n")
	if strings.Contains(re.Error(), "stack traceback:") {
		t.Errorf("expected no traceback for a main-chunk-only stack, got:\n%s", re.Error())
	}
	if re.Error() != re.Message() {
		t.Errorf("Error() = %q, want it to equal Message() %q", re.Error(), re.Message())
	}
}

// Function literals bound to a name are named after it, so a traceback says
// what a reader would call the function rather than "anon@<line>".
func TestTracebackNamesBoundFunctionLiterals(t *testing.T) {
	re := runFailing(t, "demo.lsc", `
local M = {}
M.handler = function(t)
    return t.x
end
local runIt = function(t)
    return M.handler(t)
end
function M.entry(t)
    return runIt(t)
end
M.entry(nil)
`)
	wantLines(t, re.Error(),
		"in function 'M.handler'",
		"in function 'runIt'",
		"in function 'M.entry'",
		"in main chunk",
	)
}

// A genuinely anonymous literal still locates its definition.
func TestAnonymousFunctionKeepsDefinitionSite(t *testing.T) {
	re := runFailing(t, "demo.lsc", `
local fns = { function(t) return t.x end }
fns[1](nil)
`)
	if !strings.Contains(re.Error(), "in function 'anon@2'") {
		t.Errorf("expected anon@2 in:\n%s", re.Error())
	}
}

// assert's own default message is positioned like any other builtin error,
// while a message the caller supplies is the error object and stays verbatim.
func TestAssertMessagePositioning(t *testing.T) {
	re := runFailing(t, "demo.lsc", "\nassert(1 == 2)\n")
	if want := "demo.lsc:2: assertion failed!"; re.Message() != want {
		t.Errorf("message = %q, want %q", re.Message(), want)
	}

	v := run(t, `ok, err = pcall(function() assert(false, "custom") end)`)
	if got := global(t, v, "err"); !Equal(got, "custom") {
		t.Errorf("err = %v, want the supplied message verbatim", got)
	}
}

// The VM's own faults are reported at the script position that provoked
// them rather than as an opaque "vm panic".
func TestRuntimeErrorExposesRaisedValue(t *testing.T) {
	re := runFailing(t, "demo.lsc", `error("raw value")`)
	if got, ok := re.Value.(string); !ok || got != "demo.lsc:1: raw value" {
		t.Errorf("Value = %v (%T), want the positioned message string", re.Value, re.Value)
	}
}
