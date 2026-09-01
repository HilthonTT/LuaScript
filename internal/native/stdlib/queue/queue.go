package queue

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterQueuePreload(v *vm.VM) {
	vm.RegisterPreload(v, "queue", queueLoader)
}

func queueLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 2)
	mod.Set("VERSION", "0.1.0")

	methods := vm.NewTable(0, 4)

	methods.Set("new", &vm.GoFunc{Name: "queue:new", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		var opts *vm.Table
		if len(args) > 0 && args[0] != nil {
			opts = vm.TableArg("queue.new", 1, args)
		}
		capacity := 0
		var onError vm.Value
		if opts != nil {
			capacity = int(optIntField(opts, "capacity", 0))
			if f := opts.Get("on_error"); f != nil {
				onError = callableField("queue.new", "on_error", f)
			}
		}
		return []vm.Value{wrapQueue(NewDispatcher(capacity), onError)}
	}})

	methods.Set("channel", &vm.GoFunc{Name: "queue:channel", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		capacity := vm.OptInt("queue.channel", 1, args, 0)
		return []vm.Value{wrapChannel(NewChannel(int(capacity)), nil)}
	}})

	methods.Set("after", &vm.GoFunc{Name: "queue:after", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		ms := vm.IntArg("queue.after", 1, args)
		var val vm.Value = true
		if len(args) > 1 && args[1] != nil {
			val = args[1]
		}
		c := NewChannel(1)
		go func() {
			t := time.NewTimer(durationMS(ms))
			defer t.Stop()
			select {
			case <-t.C:
				c.Send(val, 0)
				c.Close()
			case <-c.done:
			}
		}()
		return []vm.Value{wrapChannel(c, nil)}
	}})

	methods.Set("tick", &vm.GoFunc{Name: "queue:tick", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		ms := vm.IntArg("queue.tick", 1, args)
		if ms <= 0 {
			panic(vm.Errorf("bad argument #1 to 'queue.tick' (interval must be > 0, got %d)", ms))
		}
		c := NewChannel(1)
		go func() {
			t := time.NewTicker(durationMS(ms))
			defer t.Stop()
			for {
				select {
				case <-t.C:
					c.Send(true, 0)
				case <-c.done:
					return
				}
			}
		}()
		return []vm.Value{wrapChannel(c, c.Close)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	mod.SetMetatable(mt)
	return []vm.Value{mod}
}

func wrapQueue(d *Dispatcher, onError vm.Value) *vm.Table {
	o := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 8)

	var nextID uint64

	methods.Set("push", &vm.GoFunc{Name: "queue:push", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:push", 1, args)
		fn := funcArg("queue:push", 2, args)

		var opts *vm.Table
		if len(args) > 2 && args[2] != nil {
			opts = vm.TableArg("queue:push", 3, args)
		}

		j := &Job{Fn: fn}
		if opts != nil {
			j.Priority = optIntField(opts, "priority", 0)
			j.Retries = int(optIntField(opts, "retries", 0))
			j.Timeout = durationMS(optIntField(opts, "timeout_ms", 0))
			j.Backoff = durationMS(optIntField(opts, "backoff_ms", 0))
			j.Payload = opts.Get("payload")
			if d := optIntField(opts, "delay_ms", 0); d > 0 {
				j.ReadyAt = time.Now().Add(durationMS(d))
			}
			if id, ok := opts.Get("id").(string); ok {
				j.ID = id
			}
			if t, ok := opts.Get("args").(*vm.Table); ok {
				n := t.Len()
				j.Args = make([]vm.Value, 0, n)
				for i := int64(1); i <= n; i++ {
					j.Args = append(j.Args, t.Get(i))
				}
			}
		}
		if j.ID == "" {
			nextID++
			j.ID = "job-" + strconv.FormatUint(nextID, 10)
		}

		if err := d.Submit(j); err != nil {
			return []vm.Value{nil, err.Error()}
		}
		return []vm.Value{j.ID}
	}})

	methods.Set("run", &vm.GoFunc{Name: "queue:run", Fn: func(machine *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:run", 1, args)
		return []vm.Value{pump(machine, d, onError, true, 0)}
	}})

	methods.Set("poll", &vm.GoFunc{Name: "queue:poll", Fn: func(machine *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:poll", 1, args)
		max := vm.OptInt("queue:poll", 2, args, 0)
		return []vm.Value{pump(machine, d, onError, false, max)}
	}})

	methods.Set("stop", &vm.GoFunc{Name: "queue:stop", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:stop", 1, args)
		d.Stop()
		return nil
	}})
	methods.Set("is_stopped", &vm.GoFunc{Name: "queue:is_stopped", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:is_stopped", 1, args)
		return []vm.Value{d.Stopped()}
	}})
	methods.Set("size", &vm.GoFunc{Name: "queue:size", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:size", 1, args)
		return []vm.Value{int64(d.Len())}
	}})
	methods.Set("empty", &vm.GoFunc{Name: "queue:empty", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:empty", 1, args)
		return []vm.Value{d.Len() == 0}
	}})
	methods.Set("clear", &vm.GoFunc{Name: "queue:clear", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:clear", 1, args)
		return []vm.Value{int64(d.Clear())}
	}})
	methods.Set("metrics", &vm.GoFunc{Name: "queue:metrics", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("queue:metrics", 1, args)
		return []vm.Value{metricsTable(d)}
	}})

	mt := vm.NewTable(0, 3)
	mt.Set("__index", methods)
	mt.Set("__len", &vm.GoFunc{Name: "queue:__len", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(d.Len())}
	}})
	mt.Set("__tostring", &vm.GoFunc{Name: "queue:__tostring", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if d.Stopped() {
			return []vm.Value{fmt.Sprintf("queue(pending=%d, stopped)", d.Len())}
		}
		return []vm.Value{fmt.Sprintf("queue(pending=%d)", d.Len())}
	}})
	o.SetMetatable(mt)
	return o
}

func pump(machine *vm.VM, d *Dispatcher, onError vm.Value, blocking bool, max int64) int64 {
	var processed int64

	for {
		if max > 0 && processed >= max {
			return processed
		}

		now := time.Now()
		job, wait, ok := d.NextDue(now)
		if !ok {
			if !blocking || wait <= 0 {
				return processed
			}
			t := time.NewTimer(wait)
			select {
			case <-t.C:
			case <-d.Wake():
			}
			t.Stop()
			continue
		}

		if job.Expired(now) {
			d.MarkExpired()
			report(machine, onError, job, fmt.Sprintf("job %q expired before it ran", job.ID), true)
			continue
		}

		waited := now.Sub(job.ReadyAt)
		if waited < 0 {
			waited = 0
		}

		start := time.Now()
		_, errVal, failed := machine.SafeCall(job.Fn, job.Args)
		exec := time.Since(start)

		if d.Complete(job, failed, waited, exec) == Failed {
			report(machine, onError, job, vm.ToString(errVal), false)
		}
		processed++

		if d.Stopped() {
			return processed
		}
	}
}

func report(machine *vm.VM, onError vm.Value, job *Job, msg string, expired bool) {
	if onError == nil {
		return
	}
	info := vm.NewTable(0, 5)
	info.Set("id", job.ID)
	info.Set("priority", job.Priority)
	info.Set("attempts", int64(job.Attempts))
	info.Set("retries", int64(job.Retries))
	info.Set("expired", expired)
	if job.Payload != nil {
		info.Set("payload", job.Payload)
	}

	if _, errVal, failed := machine.SafeCall(onError, []vm.Value{msg, info}); failed {
		fmt.Fprintf(os.Stderr, "queue: on_error handler failed: %s\n", vm.ToString(errVal))
	}
}

func metricsTable(d *Dispatcher) *vm.Table {
	m := d.Snapshot()
	t := vm.NewTable(0, 12)
	t.Set("enqueued", m.Enqueued)
	t.Set("processed", m.Processed)
	t.Set("succeeded", m.Succeeded)
	t.Set("failed", m.Failed)
	t.Set("retried", m.Retried)
	t.Set("expired", m.Expired)
	t.Set("dropped", m.Dropped)
	t.Set("pending", int64(d.Len()))
	t.Set("avg_wait_ms", millis(m.AvgWait()))
	t.Set("avg_exec_ms", millis(m.AvgExec()))
	t.Set("max_wait_ms", millis(m.MaxWait))
	t.Set("max_exec_ms", millis(m.MaxExec))
	return t
}

func wrapChannel(c *Channel, stop func()) *vm.Table {
	o := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 8)

	methods.Set("send", &vm.GoFunc{Name: "channel:send", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:send", 1, args)
		v := sendValue("channel:send", args)
		timeout := time.Duration(-1)
		if len(args) > 2 && args[2] != nil {
			timeout = durationMS(vm.IntArg("channel:send", 3, args))
		}
		switch c.Send(v, timeout) {
		case OK:
			return []vm.Value{true}
		case Closed:
			panic(vm.Errorf("channel:send: send on closed channel"))
		default:
			return []vm.Value{false, "timeout"}
		}
	}})

	methods.Set("try_send", &vm.GoFunc{Name: "channel:try_send", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:try_send", 1, args)
		v := sendValue("channel:try_send", args)
		switch c.Send(v, 0) {
		case OK:
			return []vm.Value{true}
		case Closed:
			return []vm.Value{false, "closed"}
		default:
			return []vm.Value{false, "full"}
		}
	}})

	methods.Set("receive", &vm.GoFunc{Name: "channel:receive", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:receive", 1, args)
		timeout := time.Duration(-1)
		if len(args) > 1 && args[1] != nil {
			timeout = durationMS(vm.IntArg("channel:receive", 2, args))
		}
		return receiveResults(c.Receive(timeout))
	}})

	methods.Set("try_receive", &vm.GoFunc{Name: "channel:try_receive", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:try_receive", 1, args)
		return receiveResults(c.Receive(0))
	}})

	methods.Set("close", &vm.GoFunc{Name: "channel:close", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:close", 1, args)
		c.Close()
		return nil
	}})
	methods.Set("is_closed", &vm.GoFunc{Name: "channel:is_closed", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:is_closed", 1, args)
		return []vm.Value{c.IsClosed()}
	}})
	methods.Set("len", &vm.GoFunc{Name: "channel:len", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:len", 1, args)
		return []vm.Value{int64(c.Len())}
	}})
	methods.Set("cap", &vm.GoFunc{Name: "channel:cap", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		_ = vm.TableArg("channel:cap", 1, args)
		return []vm.Value{int64(c.Cap())}
	}})
	if stop != nil {
		methods.Set("stop", &vm.GoFunc{Name: "channel:stop", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
			_ = vm.TableArg("channel:stop", 1, args)
			stop()
			return nil
		}})
	}

	mt := vm.NewTable(0, 3)
	mt.Set("__index", methods)
	mt.Set("__len", &vm.GoFunc{Name: "channel:__len", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(c.Len())}
	}})
	mt.Set("__tostring", &vm.GoFunc{Name: "channel:__tostring", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		if c.IsClosed() {
			return []vm.Value{fmt.Sprintf("channel(%d/%d, closed)", c.Len(), c.Cap())}
		}
		return []vm.Value{fmt.Sprintf("channel(%d/%d)", c.Len(), c.Cap())}
	}})
	o.SetMetatable(mt)
	return o
}

func receiveResults(v vm.Value, r Result) []vm.Value {
	if r == OK {
		return []vm.Value{v, true}
	}
	return []vm.Value{nil, false, r.String()}
}

func sendValue(site string, args []vm.Value) vm.Value {
	if len(args) < 2 {
		panic(vm.Errorf("bad argument #1 to '%s' (value expected)", site))
	}
	if args[1] == nil {
		panic(vm.Errorf("bad argument #1 to '%s' (cannot send nil; use false or a sentinel)", site))
	}
	return args[1]
}

func funcArg(site string, n int, args []vm.Value) vm.Value {
	if n < 1 || n > len(args) {
		panic(vm.Errorf("bad argument #%d to '%s' (function expected)", n, site))
	}
	switch args[n-1].(type) {
	case *vm.Closure, *vm.GoFunc:
		return args[n-1]
	}
	panic(vm.Errorf("bad argument #%d to '%s' (function expected, got %s)", n, site, vm.TypeName(args[n-1])))
}

func callableField(site, field string, v vm.Value) vm.Value {
	switch v.(type) {
	case *vm.Closure, *vm.GoFunc:
		return v
	}
	panic(vm.Errorf("bad option '%s' to '%s' (function expected, got %s)", field, site, vm.TypeName(v)))
}

func optIntField(t *vm.Table, field string, dflt int64) int64 {
	v := t.Get(field)
	if v == nil {
		return dflt
	}
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		if n != float64(int64(n)) {
			panic(vm.Errorf("bad option '%s' (integer expected, got float %g)", field, n))
		}
		return int64(n)
	}
	panic(vm.Errorf("bad option '%s' (number expected, got %s)", field, vm.TypeName(v)))
}

func durationMS(ms int64) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func millis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
