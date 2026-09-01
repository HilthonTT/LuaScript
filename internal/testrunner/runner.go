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

const Suffix = "_test.lsc"

type Options struct {
	Paths           []string
	Filter          string
	Verbose         bool
	FailFast        bool
	List            bool
	Color           bool
	Out             io.Writer
	RegisterNatives func(*vm.VM)
}

type FileResult struct {
	Path     string
	Results  []testx.Result
	Err      error
	Aborted  bool
	Duration time.Duration
}

type Summary struct {
	Files      int
	Passed     int
	Failed     int
	Skipped    int
	FileErrors int
	Duration   time.Duration
}

func (s Summary) OK() bool { return s.Failed == 0 && s.FileErrors == 0 }

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
		if opts.FailFast && (fr.Err != nil || fr.Aborted) {
			break
		}
	}
	sum.Duration = time.Since(start)
	rep.summary(sum)
	return sum, nil
}

func searchPaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	return paths
}

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

func skipDir(path, root, name string) bool {
	if path == root {
		return false
	}
	if name == pkgmanager.ModulesDir || name == "node_modules" {
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func runFile(path string, opts Options) (fr FileResult) {
	fr = FileResult{Path: path}
	start := time.Now()
	defer func() { fr.Duration = time.Since(start) }()

	src, err := os.ReadFile(path)
	if err != nil {
		fr.Err = err
		return fr
	}
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
