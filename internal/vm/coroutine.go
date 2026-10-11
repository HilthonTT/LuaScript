package vm

type Thread struct {
	Stack     []Value
	Frames    []*CallFrame
	OpenUpvs  []*Upvalue
	CallMarks []int
}

type Coroutine struct {
	fn       Value
	thread   *Thread
	resumeCh chan []Value
	yieldCh  chan yieldMsg
	status   string
	started  bool

	// isMain marks the object coroutine.running() hands out for the main
	// thread. It has no goroutine; its status is derived from whether any
	// coroutine is currently running (see coroutineStatus).
	isMain bool

	// deathErr holds the error a coroutine died with, so coroutine.close
	// can report it as `false, err` (Lua 5.4). Cleared once reported.
	diedWithError bool
	deathErr      Value
}

type yieldMsg struct {
	values []Value
	done   bool
	failed bool
	errVal Value
}

type closeSignal struct{}

func isCloseSignal(r any) bool {
	_, ok := r.(closeSignal)
	return ok
}

func newCoroutine(fn Value) *Coroutine {
	return &Coroutine{
		fn:       fn,
		thread:   &Thread{},
		resumeCh: make(chan []Value),
		yieldCh:  make(chan yieldMsg),
		status:   "suspended",
	}
}

func (co *Coroutine) goroutineBody(v *VM) {
	defer func() {
		if r := recover(); r != nil {
			if isCloseSignal(r) {
				// coroutine.close abandons the coroutine mid-flight; its
				// suspended frames still owe their `<close>` variables.
				v.closePendingTBC(nil)
				co.yieldCh <- yieldMsg{done: true, failed: true, errVal: closeSignal{}}
				return
			}
			errVal := v.errorValue(r)
			v.closePendingTBC(errVal)
			co.yieldCh <- yieldMsg{done: true, failed: true, errVal: errVal}
		}
	}()
	args := <-co.resumeCh
	results := v.CallValue(co.fn, args, -1)
	co.yieldCh <- yieldMsg{values: results, done: true}
}

func (v *VM) saveActiveTo(t *Thread) {
	t.Stack = v.Stack
	t.Frames = v.frames
	t.OpenUpvs = v.openUpvs
	t.CallMarks = v.callMarks
	for _, u := range t.OpenUpvs {
		u.Stack = &t.Stack
	}
}

func (v *VM) loadActiveFrom(t *Thread) {
	v.Stack = t.Stack
	v.frames = t.Frames
	v.openUpvs = t.OpenUpvs
	v.callMarks = t.CallMarks
	for _, u := range v.openUpvs {
		u.Stack = &v.Stack
	}
}

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

// functionArg accepts a Lua closure or a Go function. Like Lua 5.4's
// luaL_checktype(L, n, LUA_TFUNCTION), callable tables are rejected here.
func functionArg(name string, n int, args []Value) Value {
	if n < 1 || n > len(args) {
		panic(Errorf("bad argument #%d to '%s' (function expected)", n, name))
	}
	switch fn := args[n-1].(type) {
	case *Closure, *GoFunc:
		return fn
	}
	panic(Errorf("bad argument #%d to '%s' (function expected, got %s)", n, name, describeBadArg(args[n-1])))
}

// mainCoroutine returns the lazily created object that stands for the main
// thread, so coroutine.running() from top-level code has something to return.
func (v *VM) mainCoroutine() *Coroutine {
	if v.mainCo == nil {
		v.mainCo = &Coroutine{isMain: true, status: "running", started: true}
	}
	return v.mainCo
}

// coroutineStatus reports co's Lua status. The main thread is "running" when
// no coroutine is active and "normal" while it waits on a resumed coroutine.
func (v *VM) coroutineStatus(co *Coroutine) string {
	if co.isMain {
		if v.currentCo == nil {
			return "running"
		}
		return "normal"
	}
	return co.status
}

func builtinCoroutineCreate(v *VM, args []Value) []Value {
	fn := functionArg("create", 1, args)
	return []Value{newCoroutine(fn)}
}

func builtinCoroutineResume(v *VM, args []Value) []Value {
	co := CoroutineArg("resume", 1, args)
	switch v.coroutineStatus(co) {
	case "dead":
		return []Value{false, "cannot resume dead coroutine"}
	case "running", "normal":
		return []Value{false, "cannot resume non-suspended coroutine"}
	}

	prev := v.currentCo
	prevThread := v.activeThread()
	v.saveActiveTo(prevThread)
	v.loadActiveFrom(co.thread)
	v.currentCo = co
	co.status = "running"
	if prev != nil {
		// The resumer is suspended on us, not yielded: Lua calls that "normal".
		prev.status = "normal"
	}

	if !co.started {
		co.started = true
		go co.goroutineBody(v)
	}

	co.resumeCh <- args[1:]
	msg := <-co.yieldCh

	v.saveActiveTo(co.thread)
	v.loadActiveFrom(prevThread)
	v.currentCo = prev
	if prev != nil {
		prev.status = "running"
	}

	if msg.done {
		co.status = "dead"
	} else {
		co.status = "suspended"
	}

	if msg.failed {
		co.diedWithError = true
		co.deathErr = msg.errVal
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
	if len(resumeArgs) == 1 && isCloseSignal(resumeArgs[0]) {
		panic(closeSignal{})
	}
	return resumeArgs
}

func builtinCoroutineStatus(v *VM, args []Value) []Value {
	co := CoroutineArg("status", 1, args)
	return []Value{v.coroutineStatus(co)}
}

func builtinCoroutineWrap(v *VM, args []Value) []Value {
	fn := functionArg("wrap", 1, args)
	co := newCoroutine(fn)
	wrapper := &GoFunc{
		Name: "coroutine.wrap:fn",
		Fn: func(vm *VM, callArgs []Value) []Value {
			results := builtinCoroutineResume(vm, append([]Value{co}, callArgs...))
			if len(results) == 0 {
				return nil
			}
			ok, _ := results[0].(bool)
			if !ok {
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

func builtinCoroutineRunning(v *VM, _ []Value) []Value {
	if v.currentCo == nil {
		return []Value{v.mainCoroutine(), true}
	}
	return []Value{v.currentCo, false}
}

func builtinCoroutineClose(v *VM, args []Value) []Value {
	co := CoroutineArg("close", 1, args)
	switch v.coroutineStatus(co) {
	case "dead":
		if co.diedWithError {
			// Lua 5.4: closing a coroutine that died in error reports that
			// error once; the coroutine is then reset to a clean dead state.
			err := co.deathErr
			co.diedWithError = false
			co.deathErr = nil
			return []Value{false, err}
		}
		return []Value{true}
	case "running":
		panic(LuaError("cannot close a running coroutine"))
	case "normal":
		panic(LuaError("cannot close a normal coroutine"))
	}
	if !co.started {
		co.status = "dead"
		return []Value{true}
	}

	prev := v.currentCo
	prevThread := v.activeThread()
	v.saveActiveTo(prevThread)
	v.loadActiveFrom(co.thread)
	v.currentCo = co
	co.status = "running"
	if prev != nil {
		prev.status = "normal"
	}

	co.resumeCh <- []Value{closeSignal{}}
	msg := <-co.yieldCh

	v.saveActiveTo(co.thread)
	v.loadActiveFrom(prevThread)
	v.currentCo = prev
	if prev != nil {
		prev.status = "running"
	}
	co.status = "dead"

	if msg.failed && !isCloseSignal(msg.errVal) {
		return []Value{false, msg.errVal}
	}
	return []Value{true}
}

func (v *VM) activeThread() *Thread {
	if v.currentCo == nil {
		return v.mainThread
	}
	return v.currentCo.thread
}
