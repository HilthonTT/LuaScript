package vm

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

func run(t *testing.T, src string) *VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := New()
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func runErr(t *testing.T, src string) string {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := New()
	e := v.Run(chunks[0])
	if e == nil {
		t.Fatalf("expected runtime error; got success\nsource:\n%s", src)
	}
	return e.Error()
}

func global(t *testing.T, v *VM, name string) Value {
	t.Helper()
	return v.Globals.Get(name)
}

func assertGlobalEqual(t *testing.T, v *VM, name string, want Value) {
	t.Helper()
	got := global(t, v, name)
	if !Equal(got, want) {
		t.Errorf("global %q = %v (%T), want %v (%T)", name, got, got, want, want)
	}
}

func TestIntegerArithmeticPreservesIntegerSubtype(t *testing.T) {
	v := run(t, `r = 2 + 3 * 4`)
	assertGlobalEqual(t, v, "r", int64(14))
}

func TestDivisionAlwaysFloat(t *testing.T) {
	v := run(t, `r = 6 / 2`)
	if got, ok := global(t, v, "r").(float64); !ok || got != 3.0 {
		t.Errorf("r = %v (%T), want 3.0 (float64)", global(t, v, "r"), global(t, v, "r"))
	}
}

func TestFloorDivisionStaysIntegerWhenBothInts(t *testing.T) {
	v := run(t, `r = 7 // 2`)
	assertGlobalEqual(t, v, "r", int64(3))
}

func TestFloorDivisionFloorsTowardNegInfinity(t *testing.T) {
	v := run(t, `r = -7 // 2`)
	assertGlobalEqual(t, v, "r", int64(-4))
}

func TestPowerAlwaysFloat(t *testing.T) {
	v := run(t, `r = 2 ^ 10`)
	if got, ok := global(t, v, "r").(float64); !ok || got != 1024.0 {
		t.Errorf("r = %v, want 1024.0", global(t, v, "r"))
	}
}

func TestModSignFollowsDivisor(t *testing.T) {
	v := run(t, `a = -1 % 3; b = 1 % -3`)
	assertGlobalEqual(t, v, "a", int64(2))
	assertGlobalEqual(t, v, "b", int64(-2))
}

func TestUnaryMinusPreservesSubtype(t *testing.T) {
	v := run(t, `a = -5; b = -5.5`)
	assertGlobalEqual(t, v, "a", int64(-5))
	if got, ok := global(t, v, "b").(float64); !ok || got != -5.5 {
		t.Errorf("b = %v, want -5.5", global(t, v, "b"))
	}
}

func TestBitwiseOps(t *testing.T) {
	v := run(t, `
		a = 0xF0 & 0x0F
		b = 0xF0 | 0x0F
		c = 0xFF ~ 0x0F
		d = 1 << 4
		e = 256 >> 4
		f = ~0
	`)
	assertGlobalEqual(t, v, "a", int64(0x00))
	assertGlobalEqual(t, v, "b", int64(0xFF))
	assertGlobalEqual(t, v, "c", int64(0xF0))
	assertGlobalEqual(t, v, "d", int64(16))
	assertGlobalEqual(t, v, "e", int64(16))
	assertGlobalEqual(t, v, "f", int64(-1))
}

func TestStringConcat(t *testing.T) {
	v := run(t, `s = "hello" .. " " .. "world"`)
	assertGlobalEqual(t, v, "s", "hello world")
}

func TestConcatNumbers(t *testing.T) {
	v := run(t, `s = "x = " .. 42`)
	assertGlobalEqual(t, v, "s", "x = 42")
}

func TestLengthOfString(t *testing.T) {
	v := run(t, `n = #"hello"`)
	assertGlobalEqual(t, v, "n", int64(5))
}

func TestNumericComparisonAcrossSubtypes(t *testing.T) {
	v := run(t, `
		a = 1 == 1.0
		b = 1 < 2.0
		c = 2.5 > 2
	`)
	assertGlobalEqual(t, v, "a", true)
	assertGlobalEqual(t, v, "b", true)
	assertGlobalEqual(t, v, "c", true)
}

func TestStringComparison(t *testing.T) {
	v := run(t, `
		a = "abc" < "abd"
		b = "abc" == "abc"
	`)
	assertGlobalEqual(t, v, "a", true)
	assertGlobalEqual(t, v, "b", true)
}

func TestAndOrShortCircuit(t *testing.T) {
	v := run(t, `
		a = false and 1
		b = nil or 7
		c = 5 and 6
	`)
	assertGlobalEqual(t, v, "a", false)
	assertGlobalEqual(t, v, "b", int64(7))
	assertGlobalEqual(t, v, "c", int64(6))
}

func TestNotOperator(t *testing.T) {
	v := run(t, `
		a = not nil
		b = not 0          -- in Lua 0 is truthy
		c = not false
		d = not "x"
	`)
	assertGlobalEqual(t, v, "a", true)
	assertGlobalEqual(t, v, "b", false)
	assertGlobalEqual(t, v, "c", true)
	assertGlobalEqual(t, v, "d", false)
}

func TestIfElseChooses(t *testing.T) {
	v := run(t, `
		if 1 < 2 then r = "yes" else r = "no" end
	`)
	assertGlobalEqual(t, v, "r", "yes")
}

func TestElseIfChain(t *testing.T) {
	v := run(t, `
		x = 2
		if x == 1 then r = "one"
		elseif x == 2 then r = "two"
		elseif x == 3 then r = "three"
		else r = "other" end
	`)
	assertGlobalEqual(t, v, "r", "two")
}

func TestWhileLoopAccumulates(t *testing.T) {
	v := run(t, `
		n = 0
		i = 1
		while i <= 5 do
			n = n + i
			i = i + 1
		end
	`)
	assertGlobalEqual(t, v, "n", int64(15))
}

func TestRepeatUntil(t *testing.T) {
	v := run(t, `
		i = 0
		repeat
			i = i + 1
		until i >= 3
	`)
	assertGlobalEqual(t, v, "i", int64(3))
}

func TestBreakExitsLoop(t *testing.T) {
	v := run(t, `
		n = 0
		for i = 1, 10 do
			if i > 3 then break end
			n = i
		end
	`)
	assertGlobalEqual(t, v, "n", int64(3))
}

func TestNumericForCountsUp(t *testing.T) {
	v := run(t, `
		s = 0
		for i = 1, 5 do s = s + i end
	`)
	assertGlobalEqual(t, v, "s", int64(15))
}

func TestNumericForWithStep(t *testing.T) {
	v := run(t, `
		s = 0
		for i = 10, 1, -2 do s = s + i end
	`)
	assertGlobalEqual(t, v, "s", int64(30))
}

func TestNumericForFloatRange(t *testing.T) {
	v := run(t, `
		c = 0
		for i = 0.0, 1.0, 0.25 do c = c + 1 end
	`)
	assertGlobalEqual(t, v, "c", int64(5))
}

func TestGenericForIpairs(t *testing.T) {
	v := run(t, `
		t = {10, 20, 30}
		s = 0
		for i, x in ipairs(t) do s = s + x end
	`)
	assertGlobalEqual(t, v, "s", int64(60))
}

func TestGotoForwardJump(t *testing.T) {
	v := run(t, `
		x = 0
		goto done
		x = 1
		::done::
	`)
	assertGlobalEqual(t, v, "x", int64(0))
}

func TestTableArrayPart(t *testing.T) {
	v := run(t, `
		t = {10, 20, 30}
		a = t[1]
		b = t[2]
		c = t[3]
		n = #t
	`)
	assertGlobalEqual(t, v, "a", int64(10))
	assertGlobalEqual(t, v, "b", int64(20))
	assertGlobalEqual(t, v, "c", int64(30))
	assertGlobalEqual(t, v, "n", int64(3))
}

func TestTableHashPart(t *testing.T) {
	v := run(t, `
		t = {x = 1, y = 2}
		a = t.x
		b = t["y"]
	`)
	assertGlobalEqual(t, v, "a", int64(1))
	assertGlobalEqual(t, v, "b", int64(2))
}

func TestTableMixedConstructor(t *testing.T) {
	v := run(t, `
		t = {10, 20, name = "hi", [99] = "ninety-nine"}
		a = t[1]
		b = t[2]
		c = t.name
		d = t[99]
	`)
	assertGlobalEqual(t, v, "a", int64(10))
	assertGlobalEqual(t, v, "b", int64(20))
	assertGlobalEqual(t, v, "c", "hi")
	assertGlobalEqual(t, v, "d", "ninety-nine")
}

func TestTableMutation(t *testing.T) {
	v := run(t, `
		t = {}
		t.x = 5
		t[2] = "two"
		a = t.x
		b = t[2]
	`)
	assertGlobalEqual(t, v, "a", int64(5))
	assertGlobalEqual(t, v, "b", "two")
}

func TestSimpleFunctionCallReturnsValue(t *testing.T) {
	v := run(t, `
		function sq(x) return x * x end
		r = sq(7)
	`)
	assertGlobalEqual(t, v, "r", int64(49))
}

func TestRecursionFactorial(t *testing.T) {
	v := run(t, `
		function fact(n)
			if n <= 1 then return 1 end
			return n * fact(n - 1)
		end
		r = fact(6)
	`)
	assertGlobalEqual(t, v, "r", int64(720))
}

func TestMultipleReturnAndAssign(t *testing.T) {
	v := run(t, `
		function pair() return 1, 2 end
		a, b = pair()
	`)
	assertGlobalEqual(t, v, "a", int64(1))
	assertGlobalEqual(t, v, "b", int64(2))
}

func TestExtraReturnValuesDiscarded(t *testing.T) {
	v := run(t, `
		function three() return 1, 2, 3 end
		a = three()
	`)
	assertGlobalEqual(t, v, "a", int64(1))
}

func TestVarargFirstValueOnly(t *testing.T) {
	v := run(t, `
		function first(...)
			local x = ...
			return x
		end
		r = first(10, 20, 30)
	`)
	assertGlobalEqual(t, v, "r", int64(10))
}

func TestClosureCapturesLocal(t *testing.T) {
	v := run(t, `
		function make()
			local n = 0
			return function()
				n = n + 1
				return n
			end
		end
		c = make()
		a = c()
		b = c()
		d = c()
	`)
	assertGlobalEqual(t, v, "a", int64(1))
	assertGlobalEqual(t, v, "b", int64(2))
	assertGlobalEqual(t, v, "d", int64(3))
}

func TestClosuresShareUpvalue(t *testing.T) {
	v := run(t, `
		function make()
			local n = 0
			local function get() return n end
			local function inc() n = n + 1 end
			return get, inc
		end
		g, i = make()
		i(); i(); i()
		r = g()
	`)
	assertGlobalEqual(t, v, "r", int64(3))
}

func TestMethodCallReceivesSelf(t *testing.T) {
	v := run(t, `
		obj = { v = 42 }
		function obj:get() return self.v end
		r = obj:get()
	`)
	assertGlobalEqual(t, v, "r", int64(42))
}

func TestPcallCatchesError(t *testing.T) {
	v := run(t, `
		ok, msg = pcall(function() error("boom") end)
	`)
	assertGlobalEqual(t, v, "ok", false)
	got := global(t, v, "msg")
	if s, isStr := got.(string); !isStr || !strings.Contains(s, "boom") {
		t.Errorf("msg = %v, want a string containing \"boom\"", got)
	}
}

func TestPcallReturnsResultsOnSuccess(t *testing.T) {
	v := run(t, `
		ok, a, b = pcall(function() return 1, 2 end)
	`)
	assertGlobalEqual(t, v, "ok", true)
	assertGlobalEqual(t, v, "a", int64(1))
	assertGlobalEqual(t, v, "b", int64(2))
}

func TestArithOnStringRaises(t *testing.T) {
	msg := runErr(t, "--!nocheck\nr = {} + 1")
	if !strings.Contains(msg, "arithmetic") {
		t.Errorf("error = %q, want it to mention arithmetic", msg)
	}
}

func TestCallNonFunctionRaises(t *testing.T) {
	msg := runErr(t, "--!nocheck\nlocal x = 1 x()")
	if !strings.Contains(msg, "call") {
		t.Errorf("error = %q, want it to mention 'call'", msg)
	}
}

func TestTypeBuiltin(t *testing.T) {
	v := run(t, `
		a = type(1)
		b = type("x")
		c = type({})
		d = type(nil)
		e = type(true)
		f = type(print)
	`)
	assertGlobalEqual(t, v, "a", "number")
	assertGlobalEqual(t, v, "b", "string")
	assertGlobalEqual(t, v, "c", "table")
	assertGlobalEqual(t, v, "d", "nil")
	assertGlobalEqual(t, v, "e", "boolean")
	assertGlobalEqual(t, v, "f", "function")
}

func TestTostringNumberFormatting(t *testing.T) {
	v := run(t, `
		a = tostring(1)
		b = tostring(1.0)
		c = tostring(true)
		d = tostring(nil)
	`)
	assertGlobalEqual(t, v, "a", "1")
	assertGlobalEqual(t, v, "b", "1.0")
	assertGlobalEqual(t, v, "c", "true")
	assertGlobalEqual(t, v, "d", "nil")
}

func TestTonumberCoercion(t *testing.T) {
	v := run(t, `
		a = tonumber("42")
		b = tonumber("3.14")
		c = tonumber("nope")
	`)
	assertGlobalEqual(t, v, "a", int64(42))
	if got, ok := global(t, v, "b").(float64); !ok || got != 3.14 {
		t.Errorf("b = %v, want 3.14", global(t, v, "b"))
	}
	assertGlobalEqual(t, v, "c", nil)
}

func TestAssertPasses(t *testing.T) {
	v := run(t, `r = assert(7)`)
	assertGlobalEqual(t, v, "r", int64(7))
}

func TestAssertFailsWithMessage(t *testing.T) {
	msg := runErr(t, `assert(false, "no good")`)
	if !strings.Contains(msg, "no good") {
		t.Errorf("error = %q, want it to contain 'no good'", msg)
	}
}

func TestPairsVisitsAllEntries(t *testing.T) {
	v := run(t, `
		t = {a = 1, b = 2, c = 3}
		s = 0
		for k, v in pairs(t) do s = s + v end
	`)
	assertGlobalEqual(t, v, "s", int64(6))
}

func TestSingleAssignSemantics(t *testing.T) {
	v := run(t, `
		local t = {}
		t.x = 1
		t["y"] = 2
		local k = "z"
		t[k] = 3
		g = t.x + t.y + t[k]

		-- the target's own sub-expressions are evaluated exactly once
		calls = 0
		local function key()
			calls = calls + 1
			return "hit"
		end
		t[key()] = 99
		hits = t.hit
	`)
	assertGlobalEqual(t, v, "g", int64(6))
	assertGlobalEqual(t, v, "calls", int64(1))
	assertGlobalEqual(t, v, "hits", int64(99))
}

func TestMultiAssignEvaluatesValuesFirst(t *testing.T) {
	v := run(t, `
		local t = {}
		local i = 1
		i, t[i] = 2, 10
		old = t[1]
		new = t[2]
		idx = i

		local a, b = 1, 2
		a, b = b, a
		swapped = a * 10 + b
	`)
	assertGlobalEqual(t, v, "old", int64(10))
	assertGlobalEqual(t, v, "new", nil)
	assertGlobalEqual(t, v, "idx", int64(2))
	assertGlobalEqual(t, v, "swapped", int64(21))
}

func TestSingleAssignClampsMultiValue(t *testing.T) {
	v := run(t, `
		local function two() return 1, 2 end
		local t = {}
		g = two()
		t.f = two()
		field = t.f
		local n = select("#", two())
		count = n
	`)
	assertGlobalEqual(t, v, "g", int64(1))
	assertGlobalEqual(t, v, "field", int64(1))
	assertGlobalEqual(t, v, "count", int64(2))
}
