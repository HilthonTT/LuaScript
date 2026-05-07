package vm

// Coroutines implement Lua's symmetric resume/yield pairing. Each coroutine
// runs on its own Go goroutine; only one goroutine executes Lua at a time
// (the VM's mutable state isn't safe for parallel access). Control transfer
// happens via two channels per coroutine:
//
//   - resumeCh — main → coroutine: values delivered by `coroutine.resume`.
//   - yieldCh  — coroutine → main: values delivered by `coroutine.yield`,
//                  plus a terminal flag and an optional error (for the
//                  closure returning normally or panicking).
//
// Each side blocks on its incoming channel between turns, so the running
// goroutine has exclusive access to the VM. Before sending on resumeCh the
// caller swaps the VM's Stack/frames/openUpvs into the coroutine's saved
// thread; on receipt of yieldCh the inverse swap happens.

// Thread is the per-coroutine stack/frame state. The active thread's fields
// always match the VM's live Stack/frames/openUpvs; on every resume/yield
// boundary we save the live state into the outgoing thread and load the
// incoming thread's state into the live fields.
type Thread struct {
	Stack    []Value
	Frames   []*CallFrame
	OpenUpvs []*Upvalue
}

// Coroutine wraps a closure plus the channel handshake needed to drive it.
// `status` follows Lua's vocabulary: "suspended" (waiting for resume),
// "running" (currently executing), "normal" (suspended because it resumed
// another coroutine — not modeled in v1), and "dead" (returned or errored).
type Coroutine struct {
	fn       *Closure
	thread   *Thread
	resumeCh chan []Value
	yieldCh  chan yieldMsg
	status   string
	started  bool
}

type yieldMsg struct {
	values []Value
	done   bool   // true → final return, goroutine has exited
	err    string // non-empty → terminated by error()
}

// newCoroutine allocates a coroutine wrapping fn. The goroutine isn't
// spawned until the first resume.
func newCoroutine(fn *Closure) *Coroutine {
	return &Coroutine{
		fn:       fn,
		thread:   &Thread{},
		resumeCh: make(chan []Value),
		yieldCh:  make(chan yieldMsg),
		status:   "suspended",
	}
}

// goroutineBody is the body of every coroutine's Go-level goroutine. It
// waits for the first resume, runs the closure, and signals done.
func (co *Coroutine) goroutineBody(v *VM) {
	defer func() {
		if r := recover(); r != nil {
			msg := ""
			switch e := r.(type) {
			case luaError:
				msg = string(e)
			case error:
				msg = e.Error()
			default:
				msg = ""
			}
			co.yieldCh <- yieldMsg{done: true, err: msg}
		}
	}()
	args := <-co.resumeCh
	results := v.CallValue(co.fn, args, -1)
	co.yieldCh <- yieldMsg{values: results, done: true}
}

// saveActiveTo copies the VM's live thread fields into t.
func (v *VM) saveActiveTo(t *Thread) {
	t.Stack = v.Stack
	t.Frames = v.frames
	t.OpenUpvs = v.openUpvs
}

// loadActiveFrom installs t's fields as the VM's live thread.
func (v *VM) loadActiveFrom(t *Thread) {
	v.Stack = t.Stack
	v.frames = t.Frames
	v.openUpvs = t.OpenUpvs
}

// ---------------------------------------------------------------------------
// coroutine.* library — installed by registerStdlib
// ---------------------------------------------------------------------------

func registerCoroutineLibrary(v *VM) {
	mod := NewTable(0, 8)
	mod.Set("create", &GoFunc{Name: "coroutine.create", Fn: builtinCoroutineCreate})
	mod.Set("resume", &GoFunc{Name: "coroutine.resume", Fn: builtinCoroutineResume})
	mod.Set("yield", &GoFunc{Name: "coroutine.yield", Fn: builtinCoroutineYield})
	mod.Set("status", &GoFunc{Name: "coroutine.status", Fn: builtinCoroutineStatus})
	mod.Set("wrap", &GoFunc{Name: "coroutine.wrap", Fn: builtinCoroutineWrap})
	mod.Set("isyieldable", &GoFunc{Name: "coroutine.isyieldable", Fn: builtinCoroutineIsyieldable})
	v.Globals.Set("coroutine", mod)
}

func builtinCoroutineCreate(_ *VM, args []Value) []Value {
	if len(args) < 1 {
		panic(luaError("bad argument #1 to 'create' (function expected)"))
	}
	cl, ok := args[0].(*Closure)
	if !ok {
		panic(errorf("bad argument #1 to 'create' (function expected, got %s)", TypeName(args[0])))
	}
	return []Value{newCoroutine(cl)}
}

func builtinCoroutineResume(v *VM, args []Value) []Value {
	if len(args) < 1 {
		panic(luaError("bad argument #1 to 'resume' (coroutine expected)"))
	}
	co, ok := args[0].(*Coroutine)
	if !ok {
		panic(errorf("bad argument #1 to 'resume' (coroutine expected, got %s)", TypeName(args[0])))
	}
	if co.status == "dead" {
		return []Value{false, "cannot resume dead coroutine"}
	}
	if co.status == "running" {
		return []Value{false, "cannot resume running coroutine"}
	}

	// Save the caller's live state (always main thread in v1: nested resume
	// from inside a coroutine isn't supported here) and switch to co's.
	prev := v.currentCo
	prevThread := v.activeThread()
	v.saveActiveTo(prevThread)
	v.loadActiveFrom(co.thread)
	v.currentCo = co
	co.status = "running"

	if !co.started {
		co.started = true
		go co.goroutineBody(v)
	}

	// Hand off; co.goroutineBody (or a parked yield) is waiting.
	co.resumeCh <- args[1:]
	msg := <-co.yieldCh

	// Restore the caller.
	v.saveActiveTo(co.thread)
	v.loadActiveFrom(prevThread)
	v.currentCo = prev

	if msg.done {
		co.status = "dead"
	} else {
		co.status = "suspended"
	}

	if msg.err != "" {
		return []Value{false, msg.err}
	}
	return append([]Value{true}, msg.values...)
}

func builtinCoroutineYield(v *VM, args []Value) []Value {
	if v.currentCo == nil {
		panic(luaError("attempt to yield from outside a coroutine"))
	}
	co := v.currentCo
	co.yieldCh <- yieldMsg{values: args}
	resumeArgs := <-co.resumeCh
	return resumeArgs
}

func builtinCoroutineStatus(_ *VM, args []Value) []Value {
	if len(args) < 1 {
		panic(luaError("bad argument #1 to 'status' (coroutine expected)"))
	}
	co, ok := args[0].(*Coroutine)
	if !ok {
		panic(errorf("bad argument #1 to 'status' (coroutine expected, got %s)", TypeName(args[0])))
	}
	return []Value{co.status}
}

// builtinCoroutineWrap returns a function that resumes the coroutine each
// time it is called. The returned function returns just the yielded values
// (no leading boolean), and re-raises any error.
func builtinCoroutineWrap(v *VM, args []Value) []Value {
	if len(args) < 1 {
		panic(luaError("bad argument #1 to 'wrap' (function expected)"))
	}
	cl, ok := args[0].(*Closure)
	if !ok {
		panic(errorf("bad argument #1 to 'wrap' (function expected, got %s)", TypeName(args[0])))
	}
	co := newCoroutine(cl)
	wrapper := &GoFunc{
		Name: "coroutine.wrap:fn",
		Fn: func(vm *VM, callArgs []Value) []Value {
			results := builtinCoroutineResume(vm, append([]Value{co}, callArgs...))
			if len(results) == 0 {
				return nil
			}
			ok, _ := results[0].(bool)
			if !ok {
				msg := "error in coroutine"
				if len(results) > 1 {
					msg = ToString(results[1])
				}
				panic(luaError(msg))
			}
			return results[1:]
		},
	}
	return []Value{wrapper}
}

func builtinCoroutineIsyieldable(v *VM, _ []Value) []Value {
	return []Value{v.currentCo != nil}
}

// activeThread returns the currently-active Thread snapshot — the main
// thread when no coroutine is running, the active coroutine's thread
// otherwise. Used by resume to know where to save the caller's state.
func (v *VM) activeThread() *Thread {
	if v.currentCo == nil {
		return v.mainThread
	}
	return v.currentCo.thread
}
