// Package logx implements the `log` native module: a leveled, timestamped
// logger usable from LuaScript. Defaults: level=INFO, output=stderr, no
// prefix. Output format mirrors most CLI tools:
//
//	2026-06-04T12:34:56 [INFO] msg parts joined with tabs
//
// State (level / output writer / prefix) is per-module — one shared logger
// per `require("log")`. A second require returns the same cached table from
// package.loaded, so the configuration persists for the rest of the
// program. This matches how every other native here behaves and keeps the
// Lua-side API a flat, free-function shape.
package logx

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

// Level orders messages from most to least severe. The constants are kept
// in this order so a single `>=` comparison gates emission.
type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// levelName / levelByName form the bidirectional map between the integer
// and the strings users see on both the input and output side.
var levelName = [...]string{
	LevelTrace: "TRACE",
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
}

var levelByName = map[string]Level{
	"trace": LevelTrace, "TRACE": LevelTrace,
	"debug": LevelDebug, "DEBUG": LevelDebug,
	"info": LevelInfo, "INFO": LevelInfo,
	"warn": LevelWarn, "WARN": LevelWarn, "warning": LevelWarn, "WARNING": LevelWarn,
	"error": LevelError, "ERROR": LevelError,
	"fatal": LevelFatal, "FATAL": LevelFatal,
}

// state holds the live logger configuration. Guarded by mu so concurrent
// httpserver handlers (which run on the VM goroutine, but `set_output`
// callers from `pcall`-protected paths may race with emit) don't tear the
// writer mid-write.
type state struct {
	mu      sync.Mutex
	level   Level
	prefix  string
	out     io.Writer
	owned   io.Closer // non-nil only when out is a file we opened
	outName string    // "stderr"/"stdout"/path — for set_output round-trip
}

// RegisterLogPreload installs the `log` module under package.preload.
func RegisterLogPreload(v *vm.VM) {
	vm.RegisterPreload(v, "log", logLoader)
}

func logLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newLog()
	mod.Set("VERSION", "0.1.0")
	return []vm.Value{mod}
}

func newLog() *vm.Table {
	s := &state{
		level:   LevelInfo,
		out:     os.Stderr,
		outName: "stderr",
	}

	m := vm.NewTable(0, 4)
	methods := vm.NewTable(0, 16)

	// Per-level emit functions. Each is a thin wrapper that picks the
	// level and forwards to s.emit; the level constants stay private to
	// this package so the Lua side never sees raw integers.
	methods.Set("trace", emitFunc(s, LevelTrace))
	methods.Set("debug", emitFunc(s, LevelDebug))
	methods.Set("info", emitFunc(s, LevelInfo))
	methods.Set("warn", emitFunc(s, LevelWarn))
	methods.Set("error", emitFunc(s, LevelError))

	// fatal logs and then exits the process with status 1. We honour the
	// level threshold like every other emit — a logger configured to
	// suppress fatals still exits, matching Go's log.Fatal semantics.
	methods.Set("fatal", &vm.GoFunc{Name: "log:fatal", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		s.emit(v, LevelFatal, args)
		os.Exit(1)
		return nil
	}})

	// log.log(level, ...) — explicit-level escape hatch for cases where
	// the level is computed at runtime. Accepts either a string ("info")
	// or integer (the same constants exposed below).
	methods.Set("log", &vm.GoFunc{Name: "log:log", Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 1 {
			panic(vm.Errorf("bad argument #1 to 'log.log' (level expected)"))
		}
		lvl, err := coerceLevel(args[0])
		if err != nil {
			panic(vm.Errorf("log.log: %s", err.Error()))
		}
		s.emit(v, lvl, args[1:])
		return nil
	}})

	methods.Set("set_level", &vm.GoFunc{Name: "log:set_level", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		if len(args) < 1 {
			panic(vm.Errorf("bad argument #1 to 'log.set_level' (level expected)"))
		}
		lvl, err := coerceLevel(args[0])
		if err != nil {
			panic(vm.Errorf("log.set_level: %s", err.Error()))
		}
		s.mu.Lock()
		s.level = lvl
		s.mu.Unlock()
		return nil
	}})

	methods.Set("get_level", &vm.GoFunc{Name: "log:get_level", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		s.mu.Lock()
		name := levelName[s.level]
		s.mu.Unlock()
		return []vm.Value{strings.ToLower(name)}
	}})

	methods.Set("set_prefix", &vm.GoFunc{Name: "log:set_prefix", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		p := vm.OptString("log.set_prefix", 1, args, "")
		s.mu.Lock()
		s.prefix = p
		s.mu.Unlock()
		return nil
	}})

	// log.set_output("stderr" | "stdout" | path). Opening a file replaces
	// the previous destination and closes any file we had previously
	// opened (we never close stderr/stdout). Returns nothing on success;
	// raises on a failed file open so the caller can pcall it.
	methods.Set("set_output", &vm.GoFunc{Name: "log:set_output", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		dest := vm.StringArg("log.set_output", 1, args)
		var w io.Writer
		var owned io.Closer
		switch dest {
		case "stderr":
			w = os.Stderr
		case "stdout":
			w = os.Stdout
		default:
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				panic(vm.Errorf("log.set_output: %s", err.Error()))
			}
			w = f
			owned = f
		}
		s.mu.Lock()
		old := s.owned
		s.out = w
		s.owned = owned
		s.outName = dest
		s.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
		return nil
	}})

	methods.Set("get_output", &vm.GoFunc{Name: "log:get_output", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		s.mu.Lock()
		name := s.outName
		s.mu.Unlock()
		return []vm.Value{name}
	}})

	// log.close — flush + close any file we own. Subsequent emits revert
	// to stderr so the module stays usable. No-op if the current output
	// is stderr/stdout.
	methods.Set("close", &vm.GoFunc{Name: "log:close", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		s.mu.Lock()
		old := s.owned
		if old != nil {
			s.out = os.Stderr
			s.owned = nil
			s.outName = "stderr"
		}
		s.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
		return nil
	}})

	// Constants exposed for symmetric set_level / log.log calls. Strings
	// are the canonical form; integers are provided for callers that
	// prefer numeric comparisons.
	levels := vm.NewTable(0, 6)
	for i := LevelTrace; i <= LevelFatal; i++ {
		levels.Set(strings.ToLower(levelName[i]), int64(i))
	}
	m.Set("LEVELS", levels)

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// emitFunc builds a GoFunc that emits at a fixed level. Factoring it out
// keeps newLog's bindings compact and centralizes the panic/return shape.
func emitFunc(s *state, lvl Level) *vm.GoFunc {
	return &vm.GoFunc{Name: "log:" + strings.ToLower(levelName[lvl]), Fn: func(v *vm.VM, args []vm.Value) []vm.Value {
		s.emit(v, lvl, args)
		return nil
	}}
}

// emit renders one record. Below-threshold calls are dropped before any
// formatting work so a noisy log.debug loop in production is cheap. The
// record is rendered fully BEFORE taking the mutex: renderArg can run a
// user __tostring metamethod, and if that metamethod logs, a lock held
// across it would deadlock the non-reentrant mutex on this goroutine. The
// mutex only guards the final Write so concurrent emits don't interleave
// partial lines.
func (s *state) emit(vmRef *vm.VM, lvl Level, args []vm.Value) {
	s.mu.Lock()
	level, prefix, out := s.level, s.prefix, s.out
	s.mu.Unlock()
	if lvl < level {
		return
	}

	var b strings.Builder
	b.WriteString(time.Now().Format("2006-01-02T15:04:05"))
	b.WriteString(" [")
	b.WriteString(levelName[lvl])
	b.WriteString("] ")
	if prefix != "" {
		b.WriteString(prefix)
		b.WriteByte(' ')
	}
	for i, a := range args {
		if i > 0 {
			b.WriteByte('\t')
		}
		b.WriteString(renderArg(vmRef, a))
	}
	b.WriteByte('\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(out, b.String())
}

// renderArg renders one log argument. Strings pass through verbatim;
// other types route through the standard Lua tostring machinery so a
// custom `__tostring` metamethod on a table behaves the same way it does
// in print().
func renderArg(vmRef *vm.VM, v vm.Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	// vm.ToStringMM handles nil/bool/number/table/__tostring uniformly.
	// Passing the live VM lets a table's __tostring fire — without it the
	// helper falls back to the type-tag string, breaking parity with print.
	return vm.ToStringMM(vmRef, v)
}

// coerceLevel accepts a string name ("info", "WARN", "warning", ...) or an
// integer 0..5. Anything else is a programmer error and raises.
func coerceLevel(v vm.Value) (Level, error) {
	switch x := v.(type) {
	case string:
		if l, ok := levelByName[x]; ok {
			return l, nil
		}
		return 0, fmt.Errorf("unknown level %q (use trace|debug|info|warn|error|fatal)", x)
	case int64:
		if x < int64(LevelTrace) || x > int64(LevelFatal) {
			return 0, fmt.Errorf("level %d out of range (0..5)", x)
		}
		return Level(x), nil
	case float64:
		i := int64(x)
		if float64(i) != x || i < int64(LevelTrace) || i > int64(LevelFatal) {
			return 0, fmt.Errorf("level %v out of range (0..5)", x)
		}
		return Level(i), nil
	}
	return 0, fmt.Errorf("level must be a string or integer, got %s", vm.TypeName(v))
}
