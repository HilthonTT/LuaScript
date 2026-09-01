package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/bytecode"
	"github.com/hilthontt/luascript/internal/compiler/debug"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/vm"
)

func runProfile(argv []string) int {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	cpuOut := fs.String("cpu", "cpu.prof", "write CPU profile to this path ('' to disable)")
	memOut := fs.String("mem", "", "write heap profile to this path ('' to disable)")
	count := fs.Int("count", 1, "run the script this many times under profiling (≥1)")
	memStats := fs.Bool("mem-stats", false, "print a memory-stats summary after the run")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	args := fs.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: luascript profile [-cpu cpu.prof] [-mem mem.prof] [-count N] [-mem-stats] script.lsc")
		return 2
	}
	if *count < 1 {
		fmt.Fprintln(os.Stderr, "luascript profile: -count must be ≥ 1")
		return 2
	}

	path := args[0]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript profile:", err)
		return 1
	}
	chunks, err := compiler.CompileToInstructions(string(src), parser.NormalMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript profile:", err)
		return 1
	}
	if len(chunks) > 0 {
		chunks[0].SetSource(path)
	}

	prof, err := debug.Start(*cpuOut, *memOut)
	if err != nil {
		fmt.Fprintln(os.Stderr, "luascript profile:", err)
		_ = prof.Stop()
		return 1
	}

	start := time.Now()
	runErr := runProfileIterations(chunks[0], path, *count)
	elapsed := time.Since(start)

	if stopErr := prof.Stop(); stopErr != nil {
		fmt.Fprintln(os.Stderr, "luascript profile:", stopErr)
		if runErr == nil {
			runErr = stopErr
		}
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "luascript profile:", runErr)
		return 1
	}

	fmt.Fprintf(os.Stderr, "luascript profile: %d run(s) in %v\n", *count, elapsed)
	if *cpuOut != "" {
		fmt.Fprintf(os.Stderr, "  cpu profile: %s\n", *cpuOut)
		fmt.Fprintf(os.Stderr, "  pgo build:   go build -pgo=%s -o luascript ./cmd\n", *cpuOut)
	}
	if *memOut != "" {
		fmt.Fprintf(os.Stderr, "  mem profile: %s\n", *memOut)
	}
	if *memStats {
		printMemStats(debug.CollectMemoryStats())
	}
	return 0
}

func runProfileIterations(chunk *bytecode.InstructionSet, path string, count int) error {
	scriptDir := ""
	if abs, err := filepath.Abs(path); err == nil {
		scriptDir = filepath.Dir(abs)
	}
	for i := range count {
		v := vm.New()
		if scriptDir != "" {
			v.AddScriptDir(scriptDir)
		}
		registerAllNatives(v)
		if err := v.Run(chunk); err != nil {
			return fmt.Errorf("iteration %d: %w", i+1, err)
		}
	}
	return nil
}

func printMemStats(s debug.MemoryStats) {
	fmt.Fprintf(os.Stderr, "memory stats:\n")
	fmt.Fprintf(os.Stderr, "  heap alloc:   %d bytes\n", s.HeapAlloc)
	fmt.Fprintf(os.Stderr, "  heap sys:     %d bytes\n", s.HeapSys)
	fmt.Fprintf(os.Stderr, "  heap inuse:   %d bytes\n", s.HeapInuse)
	fmt.Fprintf(os.Stderr, "  stack inuse:  %d bytes\n", s.StackInuse)
	fmt.Fprintf(os.Stderr, "  num gc:       %d\n", s.NumGC)
	fmt.Fprintf(os.Stderr, "  gc cpu frac:  %.4f\n", s.GCCPUFraction)
	fmt.Fprintf(os.Stderr, "  goroutines:   %d\n", s.NumGoroutines)
}
