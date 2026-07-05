// Package gctune centralizes the process-wide GC knobs that the rest of the
// runtime deliberately refuses to touch. It is the single place allowed to
// flip a global runtime setting (GOGC, the soft memory limit) so the
// dangerous mutation lives behind a small, audited API rather than scattered
// `runtime/debug` calls.
//
// It complements compiler/debug, which only *reads* GC/heap stats and
// explicitly leaves the mutating knobs "at the caller's risk". gctune is that
// caller.
//
// Two consumers use it:
//   - cmd (host-side perf flags: -gc-percent, -mem-limit), applied once at
//     startup via Apply.
//   - the Lua `collectgarbage` builtin (vm/stdlib.go), which maps Lua's option
//     strings onto Collect/Stop/Restart/SetPercent/HeapBytes.
//
// All mutating entry points are guarded by a single mutex and share package
// state, matching the fact that Go has exactly one collector per process.
package gctune

import (
	"runtime"
	"runtime/debug"
	"sync"
)

// DefaultPercent mirrors Go's default GOGC. Restart and the "remembered"
// percent fall back to it when GC has never been explicitly configured.
const DefaultPercent = 100

var (
	mu sync.Mutex
	// percent is the last enabled GOGC value, remembered so Stop()/Restart()
	// can round-trip without the caller having to re-supply it.
	percent = DefaultPercent
	running = true
)

// Options configures the process GC at startup. A zero field leaves that knob
// untouched, so callers can pass only what they want to override.
type Options struct {
	// Percent sets GOGC. Zero leaves it at the current value; a negative value
	// disables the GC entirely (equivalent to collectgarbage("stop")).
	Percent int
	// MemoryLimit sets the soft heap limit in bytes. Zero leaves the current
	// limit untouched.
	MemoryLimit int64
}

// Apply installs the requested knobs. Intended to be called once, early, from
// the host (cmd) before any script runs.
func Apply(o Options) {
	if o.Percent != 0 {
		SetPercent(o.Percent)
	}
	if o.MemoryLimit != 0 {
		SetMemoryLimit(o.MemoryLimit)
	}
}

// Collect forces a full GC cycle. Backs collectgarbage("collect").
func Collect() { runtime.GC() }

// Stop disables the collector, remembering the current percent so Restart can
// restore it. Backs collectgarbage("stop").
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if prev := debug.SetGCPercent(-1); prev >= 0 {
		percent = prev
	}
	running = false
}

// Restart re-enables the collector at the last enabled percent. Backs
// collectgarbage("restart").
func Restart() {
	mu.Lock()
	defer mu.Unlock()
	debug.SetGCPercent(percent)
	running = true
}

// SetPercent sets GOGC and returns the previous value. A negative pct disables
// the GC (and is not remembered as the restart target). Backs
// collectgarbage("setpause", n).
func SetPercent(pct int) int {
	mu.Lock()
	defer mu.Unlock()
	prev := debug.SetGCPercent(pct)
	if pct >= 0 {
		percent = pct
		running = true
	} else {
		running = false
	}
	return prev
}

// SetMemoryLimit sets the soft heap limit in bytes and returns the previous
// limit. Passing a negative value queries the current limit without changing
// it (the runtime/debug contract).
func SetMemoryLimit(bytes int64) int64 { return debug.SetMemoryLimit(bytes) }

// IsRunning reports whether the collector is currently enabled. Backs
// collectgarbage("isrunning").
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

// HeapBytes returns the bytes of live heap objects currently allocated. Backs
// collectgarbage("count"). Note: ReadMemStats briefly stops the world, so this
// is a reporting call, not a hot-path metric.
func HeapBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}
