//go:build (linux || darwin || freebsd) && cgo

// Real plugin backend: generate Go source, compile it with
// `go build -buildmode=plugin`, and load the resulting .so.
//
// Go's plugin package is only implemented on linux/darwin/freebsd and needs
// cgo, hence the build constraint. Everywhere else (Windows, most notably)
// backend_stub.go supplies the same four symbols and every entry point fails
// with an explanation. Keeping the constraint here rather than at the module
// level is what lets `require("plugin")` resolve on all platforms so scripts
// can branch on `plugin.supported`.
package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goplugin "plugin" // aliased: the enclosing package is also called plugin
	"reflect"
	"regexp"
	"runtime"
	"sync"
)

const pluginSupported = true

func unsupportedReason() string { return "" }

// loadedPlugin is one opened .so.
type loadedPlugin struct {
	path string
	p    *goplugin.Plugin
}

// Opening the same .so twice in one process is an error in Go's plugin
// package on some platforms and pointless on all of them, so opened plugins
// are memoized by path. A script re-requiring the same plugin gets the same
// handle back.
var (
	openMu  sync.Mutex
	opened  = map[string]*loadedPlugin{}
	goMinor = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
)

func openPlugin(path string) (*loadedPlugin, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	openMu.Lock()
	defer openMu.Unlock()
	if lp, ok := opened[abs]; ok {
		return lp, nil
	}

	p, err := goplugin.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", abs, err)
	}
	lp := &loadedPlugin{path: abs, p: p}
	opened[abs] = lp
	return lp, nil
}

// lookup resolves an exported symbol.
//
// The one subtlety of Go plugins: a package-level *var* — which is what the
// generated source declares, `var ToUpper = strings.ToUpper` — comes back
// from Lookup as a *pointer* to that var (**func(string) string here). It has
// to be dereferenced once before it is callable. A plugin that declares a real
// `func` instead hands the func back directly, so only the pointer-to-func
// case is unwrapped; a pointer to any other kind is left alone, since keeping
// the pointer preserves the value's pointer-receiver methods.
func (lp *loadedPlugin) lookup(name string) (reflect.Value, error) {
	sym, err := lp.p.Lookup(name)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("symbol %q not found in %s", name, lp.path)
	}
	rv := reflect.ValueOf(sym)
	if rv.Kind() == reflect.Ptr && rv.Elem().Kind() == reflect.Func {
		rv = rv.Elem()
	}
	return rv, nil
}

// buildPlugin renders the spec, compiles it, and returns the .so path.
//
// Each plugin is built in its own directory under the plugin cache, named for
// a hash of the generated source: an unchanged spec is a cache hit that skips
// the compiler entirely, and a changed one lands somewhere new instead of
// racing the old artifact. Its own directory (rather than one shared one) is
// what keeps `func main` from being declared twice in a single package.
func buildPlugin(name string, s *spec) (string, error) {
	src, err := generateSource(s)
	if err != nil {
		return "", err
	}

	dir := filepath.Join(pluginDir(), name+"-"+sourceHash(src))
	so := filepath.Join(dir, "plugin.so")
	if _, err := os.Stat(so); err == nil {
		return so, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod(name)), 0o644); err != nil {
		return "", err
	}

	// Third-party imports have to be resolved (and downloaded) before the
	// build; a stdlib-only plugin needs no network round-trip at all.
	if ext := externalImports(s); len(ext) > 0 {
		if out, err := goCmd(dir, "mod", "tidy"); err != nil {
			return "", fmt.Errorf("go mod tidy failed for %v:\n%s", ext, out)
		}
	}

	if out, err := goCmd(dir, "build", "-buildmode=plugin", "-o", "plugin.so", "main.go"); err != nil {
		return "", fmt.Errorf("go build -buildmode=plugin failed:\n%s", out)
	}
	return so, nil
}

// goCmd runs the Go toolchain in dir with cgo forced on — buildmode=plugin
// does not work without it, and a user with CGO_ENABLED=0 in their
// environment would otherwise get a baffling failure.
func goCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// goMod is the module file for a generated plugin. The `go` directive must
// name the toolchain actually running us: a plugin and its host have to be
// built by the same Go version or plugin.Open rejects the .so.
func goMod(name string) string {
	v := runtime.Version()[len("go"):]
	if !goMinor.MatchString(v) {
		// A devel/rc toolchain — fall back to the module's floor rather than
		// writing a go directive the toolchain will reject.
		v = "1.21"
	}
	return fmt.Sprintf("module luascriptplugin/%s\n\ngo %s\n", name, v)
}
