package vm

import (
	"strings"
	"testing"
)

// create / status

func TestCoroutineCreateProducesSuspendedCo(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function() end)
		s = coroutine.status(co)
		ty = type(co)
	`)
	assertGlobalEqual(t, v, "s", "suspended")
	assertGlobalEqual(t, v, "ty", "thread")
}

// resume → final return values

func TestCoroutineResumeRunsToCompletion(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function(a, b) return a + b end)
		ok, sum = coroutine.resume(co, 3, 4)
		s = coroutine.status(co)
	`)
	assertGlobalEqual(t, v, "ok", true)
	assertGlobalEqual(t, v, "sum", int64(7))
	assertGlobalEqual(t, v, "s", "dead")
}

// yield round-trip

func TestCoroutineYieldRoundTrip(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function(initial)
			local x = coroutine.yield(initial + 1)   -- yields 2 on first resume(co, 1)
			local y = coroutine.yield(x * 2)         -- yields  10 if main resumes with 5
			return y + 100                            -- final return on third resume
		end)

		ok, a = coroutine.resume(co, 1)              -- resumes; co yields initial+1 = 2
		ok2, b = coroutine.resume(co, 5)             -- co yields 5*2 = 10
		ok3, c = coroutine.resume(co, 7)             -- co returns 7+100 = 107
		s = coroutine.status(co)
	`)
	assertGlobalEqual(t, v, "a", int64(2))
	assertGlobalEqual(t, v, "b", int64(10))
	assertGlobalEqual(t, v, "c", int64(107))
	assertGlobalEqual(t, v, "s", "dead")
}

// wrap as iterator (the most common producer/consumer pattern)

func TestCoroutineWrapAsIterator(t *testing.T) {
	v := run(t, `
		gen = coroutine.wrap(function()
			coroutine.yield(10)
			coroutine.yield(20)
			coroutine.yield(30)
		end)
		a = gen()
		b = gen()
		c = gen()
	`)
	assertGlobalEqual(t, v, "a", int64(10))
	assertGlobalEqual(t, v, "b", int64(20))
	assertGlobalEqual(t, v, "c", int64(30))
}

// resuming a dead coroutine

func TestResumingDeadCoroutineFails(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function() return 1 end)
		coroutine.resume(co)
		ok, msg = coroutine.resume(co)
	`)
	assertGlobalEqual(t, v, "ok", false)
	got := global(t, v, "msg")
	if s, isStr := got.(string); !isStr || !strings.Contains(s, "dead") {
		t.Errorf("msg = %v, want a string mentioning 'dead'", got)
	}
}

// error inside coroutine surfaces as (false, msg)

func TestErrorInsideCoroutineSurfacesViaResume(t *testing.T) {
	v := run(t, `
		co = coroutine.create(function() error("kaboom") end)
		ok, msg = coroutine.resume(co)
		s = coroutine.status(co)
	`)
	assertGlobalEqual(t, v, "ok", false)
	got := global(t, v, "msg")
	if s, isStr := got.(string); !isStr || !strings.Contains(s, "kaboom") {
		t.Errorf("msg = %v, want a string mentioning 'kaboom'", got)
	}
	assertGlobalEqual(t, v, "s", "dead")
}

// yield outside any coroutine errors

func TestYieldOutsideCoroutineErrors(t *testing.T) {
	msg := runErr(t, `coroutine.yield(1)`)
	if !strings.Contains(msg, "yield") {
		t.Errorf("error = %q, want it to mention yield", msg)
	}
}

// isyieldable

func TestIsyieldable(t *testing.T) {
	v := run(t, `
		main = coroutine.isyieldable()
		co = coroutine.create(function()
			coroutine.yield(coroutine.isyieldable())
		end)
		ok, inside = coroutine.resume(co)
	`)
	assertGlobalEqual(t, v, "main", false)
	assertGlobalEqual(t, v, "inside", true)
}
