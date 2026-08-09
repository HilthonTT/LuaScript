// Package testrunner discovers and executes luascript test files — the engine
// behind `luascript test`.
//
// Each file gets its own VM. That is the isolation boundary: a test file can
// scribble on globals, install metatables, or leave a module in a strange
// state without reaching the next file. Files run sequentially and so do the
// tests within them, because Lua may only execute on one goroutine (the VM has
// no locks — see internal/native/stdlib/queue for the full statement of that
// rule). Separate VMs would in principle be safe to run in parallel, but
// native modules hold process-level state (cache directories, sockets), so
// v1 does not.
//
// The runner owns discovery, VM construction and reporting; declaring and
// executing individual tests belongs to internal/native/stdlib/testx, which
// this package drives through a testx.Registry.
package testrunner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/compiler/bccache"
	"github.com/hilthontt/luascript/internal/native/stdlib/testx"
	"github.com/hilthontt/luascript/internal/pkgmanager"
	"github.com/hilthontt/luascript/internal/vm"
)

// Suffix is the filename convention for test files, mirroring Go's _test.go.
const Suffix = "_test.lsc"

// Options configures one `luascript test` invocation.
type Options struct {
	// Paths are files and directories to search. Empty means ".".
	// A file named explicitly is run whether or not it matches Suffix.
	Paths []string
	// Filter is a Lua pattern (or plain substring) matched against each
	// test's slash-joined name; empty runs everything.
	Filter string
	// Verbose reports every test, not just failures.
	Verbose bool
	// FailFast stops a file after its first failure and skips the
	// remaining files.
	FailFast bool
	// List reports the tests that would run without running them.
	List bool
	// Color enables ANSI colouring of the report.
	Color bool
	// Out receives the report. Defaults to os.Stdout.
	Out io.Writer
	// RegisterNatives installs the bundled native modules on each VM. The
	// registrar list lives in package main, so the caller supplies it —
	// the same seam repl.AddPostInit uses.
	RegisterNatives func(*vm.VM)
}

// FileResult is one test file's outcome. Err is set when the file failed to
// compile, or when the chunk itself raised outside any test — in both cases
// the tests that did run are still reported.
type FileResult struct {
	Path     string
	Results  []testx.Result
	Err      error
	Aborted  bool
	Duration time.Duration
}

// Summary tallies a whole run.
type Summary struct {
	Files      int
	Passed     int
	Failed     int
	Skipped    int
	FileErrors int
	Duration   time.Duration
}

// OK reports whether the run should exit zero.
func (s Summary) OK() bool { return s.Failed == 0 && s.FileErrors == 0 }

// Run discovers, executes and reports. The returned error covers only
// discovery problems; test failures are carried in the Summary so the caller
// can pick an exit code without inspecting an error string.
func Run(opts Options) (Summary, error) {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.RegisterNatives == nil {
		opts.RegisterNatives = func(*vm.VM) {}
	}

	files, err := Discover(opts.Paths)
	if err != nil {
		return Summary{}, err
	}
	if len(files) == 0 {
		return Summary{}, fmt.Errorf("no %s files found in %s", Suffix, strings.Join(searchPaths(opts.Paths), ", "))
	}

	rep := newReporter(opts)
	start := time.Now()
	var sum Summary
	for _, path := range files {
		fr := runFile(path, opts)
		sum.Files++
		for _, res := range fr.Results {
			switch res.Status {
			case testx.StatusPass:
				sum.Passed++
			case testx.StatusFail:
				sum.Failed++
			case testx.StatusSkip:
				sum.Skipped++
			}
		}
		if fr.Err != nil {
			sum.FileErrors++
		}
		rep.file(fr)
		// FailFast means the whole run stops, not just the file: the point
		// is to get back to the editor as fast as possible.
		if opts.FailFast && (fr.Err != nil || fr.Aborted) {
			break
		}
	}
	sum.Duration = time.Since(start)
	rep.summary(sum)
	return sum, nil
}

// searchPaths normalizes Paths for messages.
func searchPaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	return paths
}

// Discover expands paths into a sorted, deduplicated list of test files.
// Directories are walked recursively; an explicitly named file is taken as-is,
// so a one-off file that does not follow the naming convention can still be
// run.
func Discover(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		clean := filepath.Clean(p)
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}

	for _, p := range searchPaths(paths) {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDir(path, p, d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), Suffix) {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// skipDir prunes directories that cannot hold first-party tests: VCS and tool
// metadata, and the package manager's install root — a dependency's own test
// suite is its problem, not this project's.
func skipDir(path, root, name string) bool {
	if path == root {
		return false
	}
	if name == pkgmanager.ModulesDir || name == "node_modules" {
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// runFile compiles and runs one test file in a fresh VM. The result is named
// so the deferred timing lands on the value actually returned.
func runFile(path string, opts Options) (fr FileResult) {
	fr = FileResult{Path: path}
	start := time.Now()
	defer func() { fr.Duration = time.Since(start) }()

	src, err := os.ReadFile(path)
	if err != nil {
		fr.Err = err
		return fr
	}
	// Same compile path as `luascript <file>`: an unchanged test file skips
	// the whole front end on re-runs, which is most of what makes a
	// re-run-on-save loop feel instant.
	main, err := bccache.CompileCached(string(src))
	if err != nil {
		fr.Err = err
		return fr
	}
	main.SetSource(path)

	v := vm.New()
	if abs, aerr := filepath.Abs(path); aerr == nil {
		v.AddScriptDir(filepath.Dir(abs))
	}
	opts.RegisterNatives(v)

	// Installed after the standard registrars so this registry — with the
	// run's filter and reporting attached — replaces the default one.
	reg := testx.NewRegistry()
	reg.Filter = opts.Filter
	reg.FailFast = opts.FailFast
	reg.ListOnly = opts.List
	testx.Install(v, reg)

	if runErr := v.Run(main); runErr != nil {
		fr.Err = runErr
	}
	fr.Results = reg.Results
	fr.Aborted = reg.Aborted()
	return fr
}
