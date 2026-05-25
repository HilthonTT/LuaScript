// Package debug provides opt-in profiling helpers for the `sakura profile`
// subcommand. It is deliberately narrow — anything that flips a global
// runtime knob (GC percent, memory limit, traceback level) lives at the
// caller's risk and is not exported here.
package debug

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"
)

// Profile bundles the open files / pprof state for a profiling session.
// Returned by Start and passed to Stop. A zero Profile is safe to Stop
// (it's a no-op), which keeps the deferred-stop pattern clean even when
// Start failed partway.
type Profile struct {
	cpuFile *os.File
	memPath string
}

// Start opens cpuPath (if non-empty) and begins CPU profiling. If memPath
// is non-empty, the path is remembered and the heap profile is written on
// Stop. Returns the Profile handle even on partial failure so the caller
// can still defer Stop and clean up whichever side opened successfully.
func Start(cpuPath, memPath string) (*Profile, error) {
	p := &Profile{memPath: memPath}
	if cpuPath != "" {
		f, err := os.Create(cpuPath)
		if err != nil {
			return p, fmt.Errorf("debug: create cpu profile %q: %w", cpuPath, err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return p, fmt.Errorf("debug: start cpu profile: %w", err)
		}
		p.cpuFile = f
	}
	return p, nil
}

// Stop ends CPU profiling and writes the heap profile (if requested at
// Start). Safe to call on a nil/zero Profile.
func (p *Profile) Stop() error {
	if p == nil {
		return nil
	}
	var firstErr error
	if p.cpuFile != nil {
		pprof.StopCPUProfile()
		if err := p.cpuFile.Close(); err != nil {
			firstErr = fmt.Errorf("debug: close cpu profile: %w", err)
		}
		p.cpuFile = nil
	}
	if p.memPath != "" {
		// runtime.GC before WriteHeapProfile so the snapshot reflects
		// live objects rather than uncollected garbage from the run.
		runtime.GC()
		f, err := os.Create(p.memPath)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("debug: create mem profile %q: %w", p.memPath, err)
			}
		} else {
			if err := pprof.WriteHeapProfile(f); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("debug: write heap profile: %w", err)
			}
			if err := f.Close(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("debug: close mem profile: %w", err)
			}
		}
		p.memPath = ""
	}
	return firstErr
}

// MemoryStats is a snapshot of process memory state at one moment in time.
type MemoryStats struct {
	HeapAlloc     uint64
	HeapSys       uint64
	HeapIdle      uint64
	HeapInuse     uint64
	StackInuse    uint64
	StackSys      uint64
	MSpanInuse    uint64
	MCacheInuse   uint64
	GCCPUFraction float64
	NumGC         uint32
	LastGC        time.Time
	PauseNs       []time.Duration
	NumGoroutines int
}

// CollectMemoryStats gathers the current heap/GC/goroutine snapshot.
// Intended for one-shot reporting at the end of a profiling run.
func CollectMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var gcStats debug.GCStats
	debug.ReadGCStats(&gcStats)

	return MemoryStats{
		HeapAlloc:     m.HeapAlloc,
		HeapSys:       m.HeapSys,
		HeapIdle:      m.HeapIdle,
		HeapInuse:     m.HeapInuse,
		StackInuse:    m.StackInuse,
		StackSys:      m.StackSys,
		MSpanInuse:    m.MSpanInuse,
		MCacheInuse:   m.MCacheInuse,
		GCCPUFraction: m.GCCPUFraction,
		NumGC:         m.NumGC,
		LastGC:        time.Unix(0, int64(m.LastGC)),
		PauseNs:       gcStats.Pause,
		NumGoroutines: runtime.NumGoroutine(),
	}
}
