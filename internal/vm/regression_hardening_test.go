package vm

import (
	"strings"
	"testing"
	"time"

	"github.com/hilthontt/luascript/v3/internal/compiler"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
)

// runBounded runs src and fails the test if it does not finish in time, so a
// regression to a hang reports instead of stalling the suite.
func runBounded(t *testing.T, src string) *VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := New()
	done := make(chan error, 1)
	go func() { done <- v.Run(chunks[0]) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("vm error: %v\nsource:\n%s", err, src)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("script did not finish\nsource:\n%s", src)
	}
	return v
}

func assertGlobalContains(t *testing.T, v *VM, name, want string) {
	t.Helper()
	s, ok := v.Globals.Get(name).(string)
	if !ok || !strings.Contains(s, want) {
		t.Errorf("global %q = %v, want it to contain %q", name, v.Globals.Get(name), want)
	}
}

// Exceeding the pattern recursion limit used to read as "no match", so the
// matcher kept backtracking through an exponential search.
func TestPatternTooComplexErrors(t *testing.T) {
	v := runBounded(t, `--!nocheck
		local n = 5000
		ok, err = pcall(string.match, string.rep("a", n), string.rep("a?", n) .. string.rep("a", n))
		normal = string.match("key = value", "^(%w+)%s*=%s*(%w+)$")
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalContains(t, v, "err", "pattern too complex")
	assertGlobalEqual(t, v, "normal", "key")
}

// Lua caps width and precision at two digits; uncapped, a short format string
// expands into gigabytes of padding.
func TestFormatWidthPrecisionCapped(t *testing.T) {
	v := runBounded(t, `--!nocheck
		ok1, e1 = pcall(string.format, "%999999d", 1)
		ok2, e2 = pcall(string.format, "%.100f", 1)
		ok3, e3 = pcall(string.format, "%5.123s", "x")
		w = string.format("%99d", 1)
		p = string.format("%5.2f|%-4d|", 3.14159, 7)
	`)
	assertGlobalEqual(t, v, "ok1", false)
	assertGlobalContains(t, v, "e1", "invalid conversion '%999999d'")
	assertGlobalEqual(t, v, "ok2", false)
	assertGlobalEqual(t, v, "ok3", false)
	if s, _ := v.Globals.Get("w").(string); len(s) != 99 {
		t.Errorf("%%99d produced %d chars, want 99", len(s))
	}
	assertGlobalEqual(t, v, "p", " 3.14|7   |")
}

// Go's fmt renders non-finite floats as "+Inf"/"NaN"; C (and Lua) print
// "inf"/"nan".
func TestFormatNonFiniteFloats(t *testing.T) {
	v := runBounded(t, `--!nocheck
		a = string.format("[%f][%e][%g][%5.1f][%-6g][%+f][%E]", 1/0, -1/0, 1/0, 1/0, -1/0, 1/0, 1/0)
		b = string.format("%.3f", -(0/0))
	`)
	assertGlobalEqual(t, v, "a", "[inf][-inf][inf][  inf][-inf  ][+inf][INF]")
	assertGlobalEqual(t, v, "b", "nan")
}

// A table whose __call is itself used to recurse through CallValue forever.
func TestCallMetamethodSelfLoopErrors(t *testing.T) {
	v := runBounded(t, `--!nocheck
		local t = setmetatable({}, {})
		getmetatable(t).__call = t
		ok, err = pcall(t)
		local inner = setmetatable({}, {__call = function(self, outerObj, a, b) return a + b end})
		local outer = setmetatable({}, {__call = inner})
		chained = outer(2, 3)
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalContains(t, v, "err", "'__call' chain too long")
	assertGlobalEqual(t, v, "chained", int64(5))
}

// Each coroutine resuming a fresh coroutine of the same function used to nest
// goroutines without bound (maxCallDepth is per thread).
func TestNestedCoroutineResumeBounded(t *testing.T) {
	v := runBounded(t, `--!nocheck
		depth = 0
		local function f()
			depth = depth + 1
			local co = coroutine.create(f)
			local ok, err = coroutine.resume(co)
			if not ok then error(err, 0) end
		end
		ok, err = pcall(f)
		local function g() return coroutine.wrap(g)() end
		ok2, err2 = pcall(g)
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalContains(t, v, "err", "C stack overflow")
	assertGlobalEqual(t, v, "depth", int64(maxCoroutineNesting+1))
	assertGlobalEqual(t, v, "ok2", false)
	assertGlobalContains(t, v, "err2", "C stack overflow")
}

// A __close that yields while a dead coroutine's error unwinds used to park
// the goroutine; coroutine.close then re-panicked its close signal out of a
// deferred recover and crashed the process.
func TestYieldInCloseDuringErrorUnwind(t *testing.T) {
	v := runBounded(t, `--!nocheck
		local co = coroutine.create(function()
			local x <close> = setmetatable({}, {__close = function() coroutine.yield("y") end})
			error("e", 0)
		end)
		ok1, e1 = coroutine.resume(co)
		st = coroutine.status(co)
		ok2, e2 = coroutine.close(co)
		local co2 = coroutine.create(function()
			local x <close> = setmetatable({}, {__close = function() coroutine.yield("y") end})
			coroutine.yield(1)
		end)
		coroutine.resume(co2)
		ok3 = coroutine.close(co2)
		st2 = coroutine.status(co2)
	`)
	assertGlobalEqual(t, v, "ok1", false)
	assertGlobalEqual(t, v, "e1", "e")
	assertGlobalEqual(t, v, "st", "dead")
	assertGlobalEqual(t, v, "ok2", false)
	assertGlobalEqual(t, v, "e2", "e")
	assertGlobalEqual(t, v, "ok3", true)
	assertGlobalEqual(t, v, "st2", "dead")
}

// An uncaught error object whose __tostring raises used to panic inside the
// top-level recover and crash the process.
func TestUncaughtErrorWithFailingToString(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`--!nocheck
		error(setmetatable({}, {__tostring = function() error("boom") end}))
	`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	e := New().Run(chunks[0])
	re, ok := e.(*RuntimeError)
	if !ok {
		t.Fatalf("Run returned %T (%v), want *RuntimeError", e, e)
	}
	if re.Msg != "(error object is a table value)" {
		t.Errorf("Msg = %q", re.Msg)
	}
}

func TestPairsHonoursMetamethod(t *testing.T) {
	v := run(t, `--!nocheck
		local proxy = setmetatable({}, {__pairs = function(t)
			local i = 0
			return function() i = i + 1 if i <= 3 then return i, i * 10 end end, t, nil
		end})
		sum = 0
		for k, val in pairs(proxy) do sum = sum + k + val end
		plain = 0
		for k in pairs({1, 2, 3}) do plain = plain + k end
	`)
	assertGlobalEqual(t, v, "sum", int64(66))
	assertGlobalEqual(t, v, "plain", int64(6))
}

func TestIntArgNonIntegralMessage(t *testing.T) {
	v := run(t, `--!nocheck
		_, e1 = pcall(string.rep, "x", 1.5)
		_, e2 = pcall(string.sub, "abc", "2.5")
		_, e3 = pcall(string.rep, "x", {})
	`)
	assertGlobalContains(t, v, "e1", "number has no integer representation")
	assertGlobalContains(t, v, "e2", "number has no integer representation")
	assertGlobalContains(t, v, "e3", "number expected, got table")
}

func TestMathModfLua54(t *testing.T) {
	v := run(t, `--!nocheck
		a, b = math.modf(5)
		ta = math.type(a)
		c, d = math.modf(-1/0)
		e, f = math.modf(3.5)
	`)
	assertGlobalEqual(t, v, "a", int64(5))
	assertGlobalEqual(t, v, "ta", "integer")
	assertGlobalEqual(t, v, "b", 0.0)
	assertGlobalEqual(t, v, "d", 0.0)
	assertGlobalEqual(t, v, "e", 3.0)
	assertGlobalEqual(t, v, "f", 0.5)
}

func TestTableRemoveMoveBounds(t *testing.T) {
	v := run(t, `--!nocheck
		ok1 = pcall(table.remove, {}, 5)
		zero = table.remove({[0] = "z"}, 0)
		empty = table.remove({})
		ok2, e2 = pcall(table.move, {1, 2, 3}, 1, 3, math.maxinteger)
		local t = {1, 2, 3}
		last = table.remove(t)
		n = #t
	`)
	assertGlobalEqual(t, v, "ok1", false)
	assertGlobalEqual(t, v, "zero", "z")
	assertGlobalEqual(t, v, "empty", nil)
	assertGlobalEqual(t, v, "ok2", false)
	assertGlobalContains(t, v, "e2", "destination wrap around")
	assertGlobalEqual(t, v, "last", int64(3))
	assertGlobalEqual(t, v, "n", int64(2))
}

func TestStringByteResultCap(t *testing.T) {
	v := run(t, `--!nocheck
		ok, err = pcall(string.byte, string.rep("a", (1 << 24) + 1), 1, -1)
		n = select("#", string.byte("hello", 1, -1))
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalContains(t, v, "err", "string slice too long")
	assertGlobalEqual(t, v, "n", int64(5))
}
