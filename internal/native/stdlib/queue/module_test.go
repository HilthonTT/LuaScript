package queue_test

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/native/stdlib/queue"
	"github.com/hilthontt/luascript/internal/vm"
)

func runQueue(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	queue.RegisterQueuePreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func runQueueErr(t *testing.T, src string) string {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	queue.RegisterQueuePreload(v)
	e := v.Run(chunks[0])
	if e == nil {
		t.Fatalf("expected a runtime error; got success\nsource:\n%s", src)
	}
	return e.Error()
}

func TestRequireResolves(t *testing.T) {
	v := runQueue(t, `
		local q = require("queue")
		ver = q.VERSION
	`)
	if got := v.Globals.Get("ver"); got != "0.1.0" {
		t.Fatalf("queue.VERSION = %v, want 0.1.0", got)
	}
}

func TestRunExecutesInPriorityOrder(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		local log = {}

		local function record(name)
			return function() log[#log + 1] = name end
		end

		q:push(record("low-1"), { priority = 1 })
		q:push(record("urgent"), { priority = 100 })
		q:push(record("low-2"), { priority = 1 })

		ran = q:run()
		order = table.concat(log, ",")
	`)
	if got := v.Globals.Get("order"); got != "urgent,low-1,low-2" {
		t.Fatalf("order = %v, want urgent,low-1,low-2", got)
	}
	if got := v.Globals.Get("ran"); got != int64(3) {
		t.Fatalf("run() = %v, want 3", got)
	}
}

func TestJobArgsAndPayload(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		q:push(function(a, b) sum = a + b end, { args = { 20, 22 } })
		q:run()
	`)
	if got := v.Globals.Get("sum"); got != int64(42) {
		t.Fatalf("sum = %v, want 42", got)
	}
}

func TestFailingJobIsIsolated(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local errs = {}
		local q = queue.new{ on_error = function(msg, info)
			errs[#errs + 1] = info.id .. ": " .. msg
		end }

		q:push(function() error("boom") end, { id = "bad" })
		q:push(function() after_bad = "ran" end, { id = "good" })
		q:run()

		reported = table.concat(errs, "|")
		local m = q:metrics()
		failed, succeeded = m.failed, m.succeeded
	`)
	if got := v.Globals.Get("after_bad"); got != "ran" {
		t.Fatalf("the job after a failing one did not run (got %v); SafeCall isolation is broken", got)
	}
	reported, _ := v.Globals.Get("reported").(string)
	if !strings.Contains(reported, "bad:") || !strings.Contains(reported, "boom") {
		t.Fatalf("on_error report = %q, want it to name the job and carry the message", reported)
	}
	if got := v.Globals.Get("failed"); got != int64(1) {
		t.Fatalf("metrics.failed = %v, want 1", got)
	}
	if got := v.Globals.Get("succeeded"); got != int64(1) {
		t.Fatalf("metrics.succeeded = %v, want 1", got)
	}
}

func TestRetriesFromLua(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		attempts = 0
		q:push(function()
			attempts = attempts + 1
			if attempts < 3 then error("not yet") end
			outcome = "ok"
		end, { retries = 5 })
		q:run()
		retried = q:metrics().retried
	`)
	if got := v.Globals.Get("attempts"); got != int64(3) {
		t.Fatalf("attempts = %v, want 3", got)
	}
	if got := v.Globals.Get("outcome"); got != "ok" {
		t.Fatalf("outcome = %v, want ok (the retry never succeeded)", got)
	}
	if got := v.Globals.Get("retried"); got != int64(2) {
		t.Fatalf("metrics.retried = %v, want 2", got)
	}
}

func TestDelayedJobRunsAfterReadyOnes(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		local log = {}
		q:push(function() log[#log + 1] = "delayed" end, { delay_ms = 20, priority = 100 })
		q:push(function() log[#log + 1] = "immediate" end, { priority = 0 })
		q:run()
		order = table.concat(log, ",")
	`)
	if got := v.Globals.Get("order"); got != "immediate,delayed" {
		t.Fatalf("order = %v, want immediate,delayed", got)
	}
}

func TestPollDoesNotWaitForDelays(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		q:push(function() ran_now = true end)
		q:push(function() ran_later = true end, { delay_ms = 5000 })

		polled = q:poll()
		still_pending = q:size()
	`)
	if got := v.Globals.Get("polled"); got != int64(1) {
		t.Fatalf("poll() = %v, want 1", got)
	}
	if got := v.Globals.Get("ran_later"); got != nil {
		t.Fatal("poll ran a job that was not due yet")
	}
	if got := v.Globals.Get("still_pending"); got != int64(1) {
		t.Fatalf("size() = %v, want 1 (the delayed job stays queued)", got)
	}
}

func TestPollMax(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		for i = 1, 5 do q:push(function() end) end
		took = q:poll(2)
		left = q:size()
	`)
	if got := v.Globals.Get("took"); got != int64(2) {
		t.Fatalf("poll(2) = %v, want 2", got)
	}
	if got := v.Globals.Get("left"); got != int64(3) {
		t.Fatalf("size() = %v, want 3", got)
	}
}

func TestCapacityBackpressure(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new{ capacity = 1 }
		id1 = q:push(function() end)
		id2, err = q:push(function() end)
	`)
	if got := v.Globals.Get("id1"); got != "job-1" {
		t.Fatalf("first push = %v, want an id", got)
	}
	if got := v.Globals.Get("id2"); got != nil {
		t.Fatalf("push over capacity = %v, want nil", got)
	}
	if got := v.Globals.Get("err"); got != "queue is full" {
		t.Fatalf("err = %v, want \"queue is full\"", got)
	}
}

func TestStopFromInsideJobHaltsRun(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		local n = 0
		q:push(function() n = n + 1; q:stop() end, { priority = 10 })
		q:push(function() n = n + 1 end)
		q:push(function() n = n + 1 end)
		ran = q:run()
		count = n
		left = q:size()
		stopped = q:is_stopped()
	`)
	if got := v.Globals.Get("ran"); got != int64(1) {
		t.Fatalf("run() = %v, want 1 (stop must halt the drain)", got)
	}
	if got := v.Globals.Get("count"); got != int64(1) {
		t.Fatalf("jobs run = %v, want 1", got)
	}
	if got := v.Globals.Get("left"); got != int64(2) {
		t.Fatalf("size() = %v, want 2 (the un-run jobs stay queued)", got)
	}
	if got := v.Globals.Get("stopped"); got != true {
		t.Fatalf("is_stopped() = %v, want true", got)
	}
}

func TestRunOnEmptyQueueReturns(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		ran = queue.new():run()
	`)
	if got := v.Globals.Get("ran"); got != int64(0) {
		t.Fatalf("run() on an empty queue = %v, want 0", got)
	}
}

func TestSelfRescheduling(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		ticks = 0
		local function tick()
			ticks = ticks + 1
			if ticks < 3 then q:push(tick, { delay_ms = 1 }) end
		end
		q:push(tick)
		q:run()
	`)
	if got := v.Globals.Get("ticks"); got != int64(3) {
		t.Fatalf("ticks = %v, want 3", got)
	}
}

func TestChannelFromLua(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local ch = queue.channel(2)
		ch:send("a")
		ch:send("b")
		size = #ch
		full, reason = ch:try_send("c")

		a, ok_a = ch:receive()
		ch:close()
		b, ok_b = ch:receive()          -- buffered value survives the close
		c, ok_c, why = ch:receive()     -- now drained
		closed = ch:is_closed()
	`)
	checks := map[string]vm.Value{
		"size": int64(2), "full": false, "reason": "full",
		"a": "a", "ok_a": true,
		"b": "b", "ok_b": true,
		"c": nil, "ok_c": false, "why": "closed",
		"closed": true,
	}
	for name, want := range checks {
		if got := v.Globals.Get(name); got != want {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
	}
}

func TestChannelReceiveTimeoutFromLua(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local ch = queue.channel(1)
		v, ok, why = ch:receive(10)
	`)
	if got := v.Globals.Get("ok"); got != false {
		t.Fatalf("ok = %v, want false", got)
	}
	if got := v.Globals.Get("why"); got != "timeout" {
		t.Fatalf("why = %v, want timeout", got)
	}
}

func TestChannelSendNilRejected(t *testing.T) {
	msg := runQueueErr(t, `
		local queue = require("queue")
		queue.channel(1):send(nil)
	`)
	if !strings.Contains(msg, "cannot send nil") {
		t.Fatalf("error = %q, want it to explain that nil cannot be sent", msg)
	}
}

func TestChannelSendOnClosedRaises(t *testing.T) {
	msg := runQueueErr(t, `
		local queue = require("queue")
		local ch = queue.channel(1)
		ch:close()
		ch:send("a")
	`)
	if !strings.Contains(msg, "closed channel") {
		t.Fatalf("error = %q, want a send-on-closed-channel error", msg)
	}
}

func TestAfter(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local ch = queue.after(10, "ding")
		v, ok = ch:receive(2000)
	`)
	if got := v.Globals.Get("ok"); got != true {
		t.Fatalf("ok = %v, want true", got)
	}
	if got := v.Globals.Get("v"); got != "ding" {
		t.Fatalf("v = %v, want ding", got)
	}
}

func TestTick(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local t = queue.tick(5)
		n = 0
		for i = 1, 3 do
			local _, ok = t:receive(2000)
			if ok then n = n + 1 end
		end
		t:stop()
		stopped = t:is_closed()
	`)
	if got := v.Globals.Get("n"); got != int64(3) {
		t.Fatalf("ticks received = %v, want 3", got)
	}
	if got := v.Globals.Get("stopped"); got != true {
		t.Fatalf("is_closed() after :stop() = %v, want true", got)
	}
}

func TestTostring(t *testing.T) {
	v := runQueue(t, `
		local queue = require("queue")
		local q = queue.new()
		q:push(function() end)
		qs = tostring(q)
		cs = tostring(queue.channel(4))
	`)
	if got := v.Globals.Get("qs"); got != "queue(pending=1)" {
		t.Errorf("tostring(queue) = %v, want queue(pending=1)", got)
	}
	if got := v.Globals.Get("cs"); got != "channel(0/4)" {
		t.Errorf("tostring(channel) = %v, want channel(0/4)", got)
	}
}

func TestPushRejectsNonFunction(t *testing.T) {
	msg := runQueueErr(t, `
		local queue = require("queue")
		queue.new():push("not a function")
	`)
	if !strings.Contains(msg, "function expected") {
		t.Fatalf("error = %q, want a \"function expected\" bad-argument error", msg)
	}
}
