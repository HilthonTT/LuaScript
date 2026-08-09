// Package testx implements the `test` module — luascript's unit-testing
// surface — and the registry that `luascript test` drives it with.
//
// # Execution model
//
// Tests run the moment they are declared, on the VM goroutine, one at a time.
// There is no separate collection phase: `test("name", fn)` invokes fn right
// there, records a Result, and returns. That falls out of the VM's own
// constraint — Lua may only run on one goroutine (see the queue module for the
// same rule stated at length) — and it buys two things. A test file behaves
// like any other chunk, so `luascript foo_test.lsc` is a legitimate way to run
// it; and `describe` needs no bookkeeping beyond a name stack, because its body
// is an ordinary call.
//
// The Registry is the only piece the host owns. `luascript test` installs one
// per file with a filter and a reporter attached; a plain script run gets a
// default registry that prints each result as it lands. Either way the module
// itself is identical, which is what keeps the two paths from drifting.
//
// # Failure reporting
//
// Every user callback — test bodies, hooks, describe bodies — is invoked
// through vm.SafeCallTrace, so a failure cannot corrupt the shared VM and the
// Lua stack at the raise point survives into the report. Assertions raise
// vm.LuaError, which the VM stamps with the source position of the calling Lua
// frame; that is where "path.lsc:14: assert_eq failed" comes from.
package testx

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

// Version is the module's reported version.
const Version = "0.1.0"

// Status is the outcome of a single test.
type Status int

const (
	StatusPass Status = iota
	StatusFail
	StatusSkip
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusFail:
		return "fail"
	case StatusSkip:
		return "skip"
	}
	return "unknown"
}

// Result is one finished test. Message and Stack are meaningful only for
// StatusFail; Stack is the rendered traceback captured at the raise point and
// is empty when the failure carried no Lua frames.
type Result struct {
	Name     string
	Status   Status
	Message  string
	Stack    string
	Duration time.Duration
}

// scope is one `describe` level: its name plus the hooks declared directly
// inside it. Hooks apply to tests in this scope and every scope nested under
// it, which is why they are read off the whole stack rather than the top.
type scope struct {
	name       string
	beforeEach []vm.Value
	afterEach  []vm.Value
}

// Registry accumulates results for one chunk and holds the knobs the host
// sets. A Registry is bound to a single VM and, like the VM, is not safe for
// concurrent use.
type Registry struct {
	// Filter, when non-empty, is a Lua pattern matched against the full
	// slash-joined test name. Non-matching tests are neither run nor
	// recorded — the same shape as `go test -run`.
	Filter string
	// FailFast stops the chunk after the first failure: later tests are
	// skipped silently rather than recorded.
	FailFast bool
	// ListOnly records every matching test as a skip without running it,
	// which is what `luascript test -list` reports.
	ListOnly bool
	// OnResult, when set, is called with each Result as it is produced.
	OnResult func(Result)

	// Results is every recorded outcome, in completion order.
	Results []Result

	scopes  []*scope
	aborted bool
}

// NewRegistry returns an empty registry with no filter and no reporter.
func NewRegistry() *Registry { return &Registry{} }

// Counts tallies the recorded results.
func (r *Registry) Counts() (pass, fail, skip int) {
	for _, res := range r.Results {
		switch res.Status {
		case StatusPass:
			pass++
		case StatusFail:
			fail++
		case StatusSkip:
			skip++
		}
	}
	return
}

// Failed reports whether any recorded test failed.
func (r *Registry) Failed() bool {
	for _, res := range r.Results {
		if res.Status == StatusFail {
			return true
		}
	}
	return false
}

// Aborted reports whether FailFast cut the chunk short.
func (r *Registry) Aborted() bool { return r.aborted }

// RegisterTestPreload installs the `test` module backed by a fresh registry
// that prints each result to stdout as it completes. This is the registrar
// listed in nativeRegistrars, so `require("test")` resolves in every VM —
// running a test file directly is a supported (if unsummarized) way to use it.
func RegisterTestPreload(v *vm.VM) {
	r := NewRegistry()
	r.OnResult = StandaloneReporter(os.Stdout)
	Install(v, r)
}

// Install installs the `test` module backed by r, replacing any module already
// registered on v. `luascript test` calls this after the standard registrars
// so its own registry wins.
func Install(v *vm.VM, r *Registry) {
	vm.RegisterPreload(v, "test", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{r.module()}
	})
}

// StandaloneReporter renders results one line each, for the case where nobody
// is going to print a summary afterwards.
func StandaloneReporter(w *os.File) func(Result) {
	return func(res Result) {
		switch res.Status {
		case StatusPass:
			fmt.Fprintf(w, "ok    %s (%s)\n", res.Name, FormatDuration(res.Duration))
		case StatusSkip:
			fmt.Fprintf(w, "skip  %s\n", res.Name)
		case StatusFail:
			fmt.Fprintf(w, "FAIL  %s\n", res.Name)
			for _, line := range strings.Split(res.Message, "\n") {
				fmt.Fprintf(w, "      %s\n", line)
			}
		}
	}
}

// FormatDuration renders a test duration compactly: sub-millisecond timings
// dominate a fast suite and "0s" tells the reader nothing.
//
// A zero duration is reported as "<1µs" rather than "0ns" — Windows' monotonic
// clock has coarse granularity and returns exactly zero for a short call, and
// claiming a test took no time at all would be a lie about the measurement,
// not a fact about the test.
func FormatDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "<1µs"
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// module builds the Lua-facing table. Members live behind __index the way the
// other native modules arrange theirs, so the docs drift audit can reflect on
// them.
func (r *Registry) module() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 24)

	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "test:" + name, Fn: fn})
	}

	// test(name, fn) / it(name, fn) — declare and immediately run one test.
	runTest := func(v *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("test.test", 1, args)
		fn := vm.AnyArg("test.test", 2, args)
		r.run(v, name, fn, false)
		return nil
	}
	set("test", runTest)
	set("it", runTest)

	// describe(name, fn) — push a name scope and run fn inside it.
	set("describe", func(v *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("test.describe", 1, args)
		fn := vm.AnyArg("test.describe", 2, args)
		r.describe(v, name, fn)
		return nil
	})

	// skip(name [, fn]) — record the test as skipped without running it.
	// The body is accepted and ignored so skipping is a one-word edit.
	set("skip", func(v *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("test.skip", 1, args)
		r.run(v, name, nil, true)
		return nil
	})

	set("before_each", func(_ *vm.VM, args []vm.Value) []vm.Value {
		fn := vm.AnyArg("test.before_each", 1, args)
		s := r.currentScope()
		s.beforeEach = append(s.beforeEach, fn)
		return nil
	})
	set("after_each", func(_ *vm.VM, args []vm.Value) []vm.Value {
		fn := vm.AnyArg("test.after_each", 1, args)
		s := r.currentScope()
		s.afterEach = append(s.afterEach, fn)
		return nil
	})

	registerAssertions(set)

	methods.Set("VERSION", Version)

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// currentScope returns the innermost describe scope, creating the implicit
// root scope on first use so top-level hooks have somewhere to live.
func (r *Registry) currentScope() *scope {
	if len(r.scopes) == 0 {
		r.scopes = append(r.scopes, &scope{})
	}
	return r.scopes[len(r.scopes)-1]
}

// qualify joins the active scope names and the test name with "/", the same
// separator `go test -run` uses, so a filter can address a whole group.
func (r *Registry) qualify(name string) string {
	parts := make([]string, 0, len(r.scopes)+1)
	for _, s := range r.scopes {
		if s.name != "" {
			parts = append(parts, s.name)
		}
	}
	if name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, "/")
}

// matches applies Filter. An empty filter matches everything; a filter that
// fails to compile as a Lua pattern would raise out of PatternFind, so it is
// matched defensively as a plain substring first.
func (r *Registry) matches(full string) bool {
	if r.Filter == "" {
		return true
	}
	if strings.Contains(full, r.Filter) {
		return true
	}
	return patternMatches(full, r.Filter)
}

// describe runs fn with name pushed onto the scope stack. A failure in the
// body itself — as opposed to inside a test — is recorded against the scope
// and swallowed, so one broken group does not take the rest of the file with
// it.
func (r *Registry) describe(v *vm.VM, name string, fn vm.Value) {
	r.scopes = append(r.scopes, &scope{name: name})
	defer func() { r.scopes = r.scopes[:len(r.scopes)-1] }()

	_, errVal, stack, failed := v.SafeCallTrace(fn, nil)
	if failed {
		r.record(Result{
			Name:    r.qualify(""),
			Status:  StatusFail,
			Message: "describe body failed: " + vm.ToStringMM(v, errVal),
			Stack:   vm.FormatTraceback(stack),
		})
	}
}

// run executes one test through the hook chain and records the outcome.
// A nil v means the caller only wants the test recorded (the skip path).
func (r *Registry) run(v *vm.VM, name string, fn vm.Value, skip bool) {
	if r.aborted {
		return
	}
	full := r.qualify(name)
	if !r.matches(full) {
		return
	}
	if skip || r.ListOnly {
		r.record(Result{Name: full, Status: StatusSkip})
		return
	}

	res := Result{Name: full, Status: StatusPass}
	start := time.Now()

	// before_each hooks run outermost-first. The first one to fail owns the
	// result and the body is skipped, but after_each still runs below —
	// cleanup has to be reachable even when setup broke.
	setupOK := true
	for _, hook := range r.hooks(true) {
		if _, errVal, stack, failed := v.SafeCallTrace(hook, nil); failed {
			res.fail(v, "before_each: ", errVal, stack)
			setupOK = false
			break
		}
	}
	if setupOK {
		if _, errVal, stack, failed := v.SafeCallTrace(fn, nil); failed {
			res.fail(v, "", errVal, stack)
		}
	}

	// after_each hooks run innermost-first and always run. A cleanup failure
	// only claims the result when the test had not already failed — the
	// original failure is the more useful one to report.
	for _, hook := range r.hooks(false) {
		if _, errVal, stack, failed := v.SafeCallTrace(hook, nil); failed && res.Status != StatusFail {
			res.fail(v, "after_each: ", errVal, stack)
		}
	}

	res.Duration = time.Since(start)
	r.record(res)
}

// fail stamps a failure onto a result, rendering the error value through
// __tostring so error(someObject) reports the way the script intended.
func (res *Result) fail(v *vm.VM, prefix string, errVal vm.Value, stack []vm.TracebackEntry) {
	res.Status = StatusFail
	res.Message = prefix + vm.ToStringMM(v, errVal)
	res.Stack = vm.FormatTraceback(stack)
}

// hooks flattens the scope stack into the hook order for one test:
// before_each outermost-first, after_each innermost-first.
func (r *Registry) hooks(before bool) []vm.Value {
	var out []vm.Value
	if before {
		for _, s := range r.scopes {
			out = append(out, s.beforeEach...)
		}
		return out
	}
	for i := len(r.scopes) - 1; i >= 0; i-- {
		s := r.scopes[i]
		for j := len(s.afterEach) - 1; j >= 0; j-- {
			out = append(out, s.afterEach[j])
		}
	}
	return out
}

// record appends a result, notifies the reporter, and honours FailFast.
func (r *Registry) record(res Result) {
	r.Results = append(r.Results, res)
	if r.OnResult != nil {
		r.OnResult(res)
	}
	if r.FailFast && res.Status == StatusFail {
		r.aborted = true
	}
}
