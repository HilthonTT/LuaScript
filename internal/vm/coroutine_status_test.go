package vm

import (
	"strings"
	"testing"
)

func TestCoroutineStatusNormalWhileResumingAnother(t *testing.T) {
	v := run(t, `
		local outer
		local inner = coroutine.create(function()
			innerSeesOuter = coroutine.status(outer)
			innerSeesSelf = coroutine.status(coroutine.running())
		end)
		outer = coroutine.create(function()
			coroutine.resume(inner)
			outerAfter = coroutine.status(outer)
		end)
		coroutine.resume(outer)
		final = coroutine.status(outer)
	`)
	assertGlobalEqual(t, v, "innerSeesOuter", "normal")
	assertGlobalEqual(t, v, "innerSeesSelf", "running")
	assertGlobalEqual(t, v, "outerAfter", "running")
	assertGlobalEqual(t, v, "final", "dead")
}

func TestCoroutineToStringIsThread(t *testing.T) {
	v := run(t, `
		mainStr = tostring(coroutine.running())
		coStr = tostring(coroutine.create(function() end))
	`)
	for _, name := range []string{"mainStr", "coStr"} {
		s, _ := global(t, v, name).(string)
		if !strings.HasPrefix(s, "thread: 0x") {
			t.Errorf("%s = %q, want a \"thread: 0x...\" string", name, s)
		}
	}
}

func TestCoroutineResumeNormalCoroutineFails(t *testing.T) {
	v := run(t, `
		local outer
		local inner = coroutine.create(function()
			ok, msg = coroutine.resume(outer)
		end)
		outer = coroutine.create(function() coroutine.resume(inner) end)
		coroutine.resume(outer)
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "msg", "cannot resume non-suspended coroutine")
}

func TestCoroutineCloseReportsDeathError(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function() error("boom", 0) end)
		coroutine.resume(co)
		ok, err = coroutine.close(co)
		ok2 = coroutine.close(co)
		okClean = coroutine.close(coroutine.create(function() end))
	`)
	assertGlobalEqual(t, v, "ok", false)
	assertGlobalEqual(t, v, "err", "boom")
	assertGlobalEqual(t, v, "ok2", true)
	assertGlobalEqual(t, v, "okClean", true)
}

func TestCoroutineRunningOnMainThread(t *testing.T) {
	v := run(t, `
		local co, ismain = coroutine.running()
		isMain = ismain
		ty = type(co)
		st = coroutine.status(co)
		local inner = coroutine.create(function() stInside = coroutine.status(co) end)
		coroutine.resume(inner)
		okClose, errClose = pcall(coroutine.close, co)
		okResume, msgResume = coroutine.resume(co)
		yieldable = coroutine.isyieldable()
	`)
	assertGlobalEqual(t, v, "isMain", true)
	assertGlobalEqual(t, v, "ty", "thread")
	assertGlobalEqual(t, v, "st", "running")
	assertGlobalEqual(t, v, "stInside", "normal")
	assertGlobalEqual(t, v, "okClose", false)
	if s, _ := v.Globals.Get("errClose").(string); !strings.Contains(s, "cannot close a running coroutine") {
		t.Fatalf("errClose = %q", s)
	}
	assertGlobalEqual(t, v, "okResume", false)
	assertGlobalEqual(t, v, "msgResume", "cannot resume non-suspended coroutine")
	assertGlobalEqual(t, v, "yieldable", false)
}

func TestCallableTablesAcceptedByHostCallSites(t *testing.T) {
	v := run(t, `
		local callable = setmetatable({}, { __call = function(self, a, b) return a + b end })
		ok, sum = pcall(callable, 2, 3)
		xok, xsum = xpcall(callable, debug and debug.traceback or print, 4, 5)
		local thrower = setmetatable({}, { __call = function() error("nope", 0) end })
		eok, emsg = pcall(thrower)
		-- Lua 5.4 requires a real function for table.sort and coroutine.create,
		-- but Go functions count as functions.
		sok = pcall(table.sort, {1, 2}, setmetatable({}, { __call = function() return false end }))
		local co = coroutine.create(select)
		cok, cval = coroutine.resume(co, 2, "a", "b")
		local gen = coroutine.wrap(tostring)
		wval = gen(5)
	`)
	assertGlobalEqual(t, v, "ok", true)
	assertGlobalEqual(t, v, "sum", int64(5))
	assertGlobalEqual(t, v, "xok", true)
	assertGlobalEqual(t, v, "xsum", int64(9))
	assertGlobalEqual(t, v, "eok", false)
	assertGlobalEqual(t, v, "emsg", "nope")
	assertGlobalEqual(t, v, "sok", false)
	assertGlobalEqual(t, v, "cok", true)
	assertGlobalEqual(t, v, "cval", "b")
	assertGlobalEqual(t, v, "wval", "5")
}

func TestPcallNonCallableStillErrors(t *testing.T) {
	v := run(t, `ok, msg = pcall({})`)
	assertGlobalEqual(t, v, "ok", false)
	if s, _ := v.Globals.Get("msg").(string); !strings.Contains(s, "attempt to call a table value") {
		t.Fatalf("msg = %q", s)
	}
}
