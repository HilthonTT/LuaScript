package gctune

import (
	"runtime"
	"runtime/debug"
	"sync"
)

const DefaultPercent = 100

var (
	mu      sync.Mutex
	percent = DefaultPercent
	running = true
)

type Options struct {
	Percent     int
	MemoryLimit int64
}

func Apply(o Options) {
	if o.Percent != 0 {
		SetPercent(o.Percent)
	}
	if o.MemoryLimit != 0 {
		SetMemoryLimit(o.MemoryLimit)
	}
}

func Collect() { runtime.GC() }

func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if prev := debug.SetGCPercent(-1); prev >= 0 {
		percent = prev
	}
	running = false
}

func Restart() {
	mu.Lock()
	defer mu.Unlock()
	debug.SetGCPercent(percent)
	running = true
}

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

func SetMemoryLimit(bytes int64) int64 {
	return debug.SetMemoryLimit(bytes)
}

func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

func HeapBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}
