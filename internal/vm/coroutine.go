package vm

type Thread struct {
	Stack     []Value
	Frames    []*CallFrame
	OpenUpvs  []*Upvalue
	CallMarks []int
}

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
	done   bool
	failed bool
	errVal Value
}

type closeSignal struct{}

func isCloseSignal(r any) bool {
	_, ok := r.(closeSignal)
	return ok
}

func newCoroutine(fn *Closure) *Coroutine {
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
				co.yieldCh <- yieldMsg{done: true, failed: true, errVal: closeSignal{}}
				return
			}
			co.yieldCh <- yieldMsg{done: true, failed: true, errVal: v.errorValue(r)}
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

	co.resumeCh <- args[1:]
	msg := <-co.yieldCh

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
	if len(resumeArgs) == 1 && isCloseSignal(resumeArgs[0]) {
		panic(closeSignal{})
	}
	return resumeArgs
}

func builtinCoroutineStatus(_ *VM, args []Value) []Value {
	co := CoroutineArg("status", 1, args)
	return []Value{co.status}
}

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
		return []Value{nil, true}
	}
	return []Value{v.currentCo, false}
}

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

func (v *VM) activeThread() *Thread {
	if v.currentCo == nil {
		return v.mainThread
	}
	return v.currentCo.thread
}
