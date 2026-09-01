package testx

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

const Version = "0.1.0"

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

type Result struct {
	Name     string
	Status   Status
	Message  string
	Stack    string
	Duration time.Duration
}

type scope struct {
	name       string
	beforeEach []vm.Value
	afterEach  []vm.Value
}

type Registry struct {
	Filter   string
	FailFast bool
	ListOnly bool
	OnResult func(Result)

	Results []Result

	scopes  []*scope
	aborted bool
}

func NewRegistry() *Registry { return &Registry{} }

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

func (r *Registry) Failed() bool {
	for _, res := range r.Results {
		if res.Status == StatusFail {
			return true
		}
	}
	return false
}

func (r *Registry) Aborted() bool { return r.aborted }

func RegisterTestPreload(v *vm.VM) {
	r := NewRegistry()
	r.OnResult = StandaloneReporter(os.Stdout)
	Install(v, r)
}

func Install(v *vm.VM, r *Registry) {
	vm.RegisterPreload(v, "test", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{r.module()}
	})
}

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

func (r *Registry) module() *vm.Table {
	m := vm.NewTable(0, 2)
	methods := vm.NewTable(0, 24)

	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "test:" + name, Fn: fn})
	}

	runTest := func(v *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("test.test", 1, args)
		fn := vm.AnyArg("test.test", 2, args)
		r.run(v, name, fn, false)
		return nil
	}
	set("test", runTest)
	set("it", runTest)

	set("describe", func(v *vm.VM, args []vm.Value) []vm.Value {
		name := vm.StringArg("test.describe", 1, args)
		fn := vm.AnyArg("test.describe", 2, args)
		r.describe(v, name, fn)
		return nil
	})

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

func (r *Registry) currentScope() *scope {
	if len(r.scopes) == 0 {
		r.scopes = append(r.scopes, &scope{})
	}
	return r.scopes[len(r.scopes)-1]
}

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

func (r *Registry) matches(full string) bool {
	if r.Filter == "" {
		return true
	}
	if strings.Contains(full, r.Filter) {
		return true
	}
	return patternMatches(full, r.Filter)
}

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

	for _, hook := range r.hooks(false) {
		if _, errVal, stack, failed := v.SafeCallTrace(hook, nil); failed && res.Status != StatusFail {
			res.fail(v, "after_each: ", errVal, stack)
		}
	}

	res.Duration = time.Since(start)
	r.record(res)
}

func (res *Result) fail(v *vm.VM, prefix string, errVal vm.Value, stack []vm.TracebackEntry) {
	res.Status = StatusFail
	res.Message = prefix + vm.ToStringMM(v, errVal)
	res.Stack = vm.FormatTraceback(stack)
}

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

func (r *Registry) record(res Result) {
	r.Results = append(r.Results, res)
	if r.OnResult != nil {
		r.OnResult(res)
	}
	if r.FailFast && res.Status == StatusFail {
		r.aborted = true
	}
}
