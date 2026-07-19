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
	// CallMarks is the pending MarkArgs stack. A thread can legitimately
	// suspend between a MarkArgs and its matching Call (any
	// `f(coroutine.yield())` shape), so the marks are per-thread state —
	// leaving them on the VM would let another thread pop them against the
	// wrong stack.
	CallMarks []int
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
	done   bool  // true → final return, goroutine has exited
	failed bool  // true → terminated by an error/panic (errVal holds it)
	errVal Value // the propagated error value when failed
}

// closeSignal is the panic value used by coroutine.close to unwind a
// suspended coroutine's goroutine. It must reach goroutineBody's terminal
// recover no matter what the coroutine body does, so every intermediate
// recover point (pcall's safeCall, try/catch's execCatching) re-panics it
// instead of treating it as a catchable Lua error.
type closeSignal struct{}

// isCloseSignal reports whether a recovered panic value is the coroutine
// close sentinel.
func isCloseSignal(r any) bool {
	_, ok := r.(closeSignal)
	return ok
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
			// Any panic — a Lua error() or a Go runtime panic (nil deref,
			// etc.) — terminates the coroutine with a failure. Carry the
			// value through so resume reports false + the real error
			// instead of silently looking like a normal completion. The
			// close sentinel is kept as-is so close can tell a clean
			// unwind from a genuine error.
			if isCloseSignal(r) {
				co.yieldCh <- yieldMsg{done: true, failed: true, errVal: closeSignal{}}
				return
			}
			co.yieldCh <- yieldMsg{done: true, failed: true, errVal: recoverValue(r)}
		}
	}()
	args := <-co.resumeCh
	results := v.CallValue(co.fn, args, -1)
	co.yieldCh <- yieldMsg{values: results, done: true}
}

// saveActiveTo copies the VM's live thread fields into t.
//
// Open upvalues point at the stack they were created over via a *[]Value.
// While a thread is live that must be &v.Stack (appends can change the
// header), but once the thread is parked v.Stack will belong to some other
// thread — so retarget its upvalues at the thread's own saved stack.
// A closure that escaped the thread (e.g. yielded out of a coroutine) then
// keeps reading the suspended thread's slots instead of whichever thread
// happens to be running.
func (v *VM) saveActiveTo(t *Thread) {
	t.Stack = v.Stack
	t.Frames = v.frames
	t.OpenUpvs = v.openUpvs
	t.CallMarks = v.callMarks
	for _, u := range t.OpenUpvs {
		u.Stack = &t.Stack
	}
}

// loadActiveFrom installs t's fields as the VM's live thread and points its
// open upvalues back at the live stack (see saveActiveTo).
func (v *VM) loadActiveFrom(t *Thread) {
	v.Stack = t.Stack
	v.frames = t.Frames
	v.openUpvs = t.OpenUpvs
	v.callMarks = t.CallMarks
	for _, u := range v.openUpvs {
		u.Stack = &v.Stack
	}
}

// coroutine.* library — installed by registerStdlib

func registerCoroutineLibrary(v *VM) {
	mod := NewTable(0, 8)
	mod.Set("create", &GoFunc{Name: "coroutine.create", Fn: builtinCoroutineCreate})
	mod.Set("resume", &GoFunc{Name: "coroutine.resume", Fn: builtinCoroutineResume})
	mod.Set("yield", &GoFunc{Name: "coroutine.yield", Fn: builtinCoroutineYield})
	mod.Set("status", &GoFunc{Name: "coroutine.status", Fn: builtinCoroutineStatus})
	mod.Set("wrap", &GoFunc{Name: "coroutine.wrap", Fn: builtinCoroutineWrap})
	mod.Set("isyieldable", &GoFunc{Name: "coroutine.isyieldable", Fn: builtinCoroutineIsyieldable})
	mod.Set("running", &GoFunc{Name: "coroutine.running", Fn: builtinCoroutineRunning})
	mod.Set("close", &GoFunc{Name: "coroutine.close", Fn: builtinCoroutineClose})
	v.Globals.Set("coroutine", mod)
}

func builtinCoroutineCreate(_ *VM, args []Value) []Value {
	cl := ClosureArg("create", 1, args)
	return []Value{newCoroutine(cl)}
}

func builtinCoroutineResume(v *VM, args []Value) []Value {
	co := CoroutineArg("resume", 1, args)
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

	if msg.failed {
		return []Value{false, msg.errVal}
	}
	return append([]Value{true}, msg.values...)
}

func builtinCoroutineYield(v *VM, args []Value) []Value {
	if v.currentCo == nil {
		panic(LuaError("attempt to yield from outside a coroutine"))
	}
	co := v.currentCo
	co.yieldCh <- yieldMsg{values: args}
	resumeArgs := <-co.resumeCh
	// coroutine.close resumes the parked yield with the close sentinel;
	// panic it up through the coroutine's frames to goroutineBody.
	if len(resumeArgs) == 1 && isCloseSignal(resumeArgs[0]) {
		panic(closeSignal{})
	}
	return resumeArgs
}

func builtinCoroutineStatus(_ *VM, args []Value) []Value {
	co := CoroutineArg("status", 1, args)
	return []Value{co.status}
}

// builtinCoroutineWrap returns a function that resumes the coroutine each
// time it is called. The returned function returns just the yielded values
// (no leading boolean), and re-raises any error.
func builtinCoroutineWrap(v *VM, args []Value) []Value {
	cl := ClosureArg("wrap", 1, args)
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
				// Re-raise the original error value unchanged.
				var ev Value = "error in coroutine"
				if len(results) > 1 {
					ev = results[1]
				}
				panic(luaError{value: ev})
			}
			return results[1:]
		},
	}
	return []Value{wrapper}
}

func builtinCoroutineIsyieldable(v *VM, _ []Value) []Value {
	return []Value{v.currentCo != nil}
}

// builtinCoroutineRunning returns the running coroutine plus a boolean that
// is true when called from the main thread. The main thread has no Coroutine
// wrapper in this VM, so the first result is nil there (the boolean is the
// reliable signal, as in Lua idiom `select(2, coroutine.running())`).
func builtinCoroutineRunning(v *VM, _ []Value) []Value {
	if v.currentCo == nil {
		return []Value{nil, true}
	}
	return []Value{v.currentCo, false}
}

// builtinCoroutineClose kills a suspended (or dead) coroutine. A started
// coroutine's goroutine is parked inside yield waiting on resumeCh; it is
// resumed with the close sentinel, which yield panics with, unwinding the
// coroutine's frames (running nothing user-visible) back to goroutineBody.
// Returns true, or false + the error if the unwind itself raised one.
func builtinCoroutineClose(v *VM, args []Value) []Value {
	co := CoroutineArg("close", 1, args)
	switch co.status {
	case "dead":
		return []Value{true}
	case "running":
		panic(LuaError("cannot close a running coroutine"))
	}
	if !co.started {
		co.status = "dead"
		return []Value{true}
	}

	// Same state-swap dance as resume: the unwinding goroutine touches the
	// VM's live Stack/frames while it runs, so those must be the coroutine's.
	prev := v.currentCo
	prevThread := v.activeThread()
	v.saveActiveTo(prevThread)
	v.loadActiveFrom(co.thread)
	v.currentCo = co
	co.status = "running"

	co.resumeCh <- []Value{closeSignal{}}
	msg := <-co.yieldCh

	v.saveActiveTo(co.thread)
	v.loadActiveFrom(prevThread)
	v.currentCo = prev
	co.status = "dead"

	if msg.failed && !isCloseSignal(msg.errVal) {
		return []Value{false, msg.errVal}
	}
	return []Value{true}
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
