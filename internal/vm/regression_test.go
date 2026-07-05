package vm

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
)

// TestIntFloatSubtypePreserved checks Lua 5.4's int/float subtype rules: an
// integral float (2.0) stays a float through arithmetic and tonumber, so
// mixed-operand results are floats, not collapsed integers.
func TestIntFloatSubtypePreserved(t *testing.T) {
	v := run(t, `
		a = math.type(2.0 + 3)
		b = math.type(2 + 3)
		c = math.type(7.0 // 2)
		d = math.type(-2.0)
		e = math.type(tonumber(3.0))
		f = tostring(2.0 + 3)
	`)
	assertGlobalEqual(t, v, "a", "float")
	assertGlobalEqual(t, v, "b", "integer")
	assertGlobalEqual(t, v, "c", "float")
	assertGlobalEqual(t, v, "d", "float")
	assertGlobalEqual(t, v, "e", "float")
	assertGlobalEqual(t, v, "f", "5.0")
}

// TestNumericForOverflowTerminates guards the for-loop overflow fix: a loop
// that steps onto math.maxinteger/mininteger must terminate, not wrap and run
// forever.
func TestNumericForOverflowTerminates(t *testing.T) {
	v := run(t, `
		up = 0
		for i = math.maxinteger - 1, math.maxinteger do up = up + 1 end
		down = 0
		for i = math.mininteger + 1, math.mininteger, -1 do down = down + 1 end
		single = 0
		for i = math.maxinteger, math.maxinteger do single = single + 1 end
		empty = 0
		for i = 5, 3 do empty = empty + 1 end
		normal = 0
		for i = 1, 5 do normal = normal + i end
	`)
	assertGlobalEqual(t, v, "up", int64(2))
	assertGlobalEqual(t, v, "down", int64(2))
	assertGlobalEqual(t, v, "single", int64(1))
	assertGlobalEqual(t, v, "empty", int64(0))
	assertGlobalEqual(t, v, "normal", int64(15))
}

// TestNumericForFloatLimitNaN — a NaN limit yields zero iterations.
func TestNumericForFloatLimitNaN(t *testing.T) {
	v := run(t, `
		n = 0
		for i = 1, 0/0 do n = n + 1 end
	`)
	assertGlobalEqual(t, v, "n", int64(0))
}

// TestTonumberWithBase covers the tonumber(s, base) path.
func TestTonumberWithBase(t *testing.T) {
	v := run(t, `
		hex = tonumber("ff", 16)
		bin = tonumber("101", 2)
		b36 = tonumber("z", 36)
		bad = tonumber("xyz", 16)
	`)
	assertGlobalEqual(t, v, "hex", int64(255))
	assertGlobalEqual(t, v, "bin", int64(5))
	assertGlobalEqual(t, v, "b36", int64(35))
	assertGlobalEqual(t, v, "bad", nil)
}

// TestPcallUpvalueErrorDoesNotCorruptVM reproduces the dangling-open-upvalue
// crash: a protected call that creates a closure (open upvalue) and then errors
// must leave the VM usable.
func TestPcallUpvalueErrorDoesNotCorruptVM(t *testing.T) {
	v := run(t, `
		ok = pcall(function()
			local captured = 999
			local inner = function() return captured end
			error("boom")
			return inner
		end)
		after = 1 + 1
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "after", int64(2))
	if n := len(v.openUpvs); n != 0 {
		t.Errorf("open upvalues after pcall = %d, want 0", n)
	}
}

// TestCoroutineRuntimeErrorReportsFailure — a runtime error inside a coroutine
// surfaces as resume returning false + the message and leaves it dead.
func TestCoroutineRuntimeErrorReportsFailure(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function()
			local t = {}
			return t.missing.deeper  -- runtime error: index a nil value
		end)
		ok, err = coroutine.resume(co)
		dead = coroutine.status(co)
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "dead", "dead")
	if s, isStr := v.Globals.Get("err").(string); !isStr || s == "" {
		t.Errorf("coroutine error = %v, want a non-empty message", v.Globals.Get("err"))
	}
}

// TestCoroutineRawPanicReportsFailure — a non-Lua Go panic (e.g. a native
// function panicking with a bare string) must surface as resume failure, not be
// swallowed into a false "success". This is the case the old "" err sentinel
// silently dropped.
func TestCoroutineRawPanicReportsFailure(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`
		co = coroutine.create(function() boom() end)
		ok, err = coroutine.resume(co)
	`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v := New()
	v.Globals.Set("boom", &GoFunc{Name: "boom", Fn: func(_ *VM, _ []Value) []Value {
		panic("raw go panic")
	}})
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("run: %v", err)
	}
	assertGlobalEqual(t, v, "ok", false)
	if s, isStr := v.Globals.Get("err").(string); !isStr || !strings.Contains(s, "raw go panic") {
		t.Errorf("coroutine error = %v, want it to contain the raw panic text", v.Globals.Get("err"))
	}
}
