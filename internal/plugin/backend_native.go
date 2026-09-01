//go:build (linux || darwin || freebsd) && cgo

package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goplugin "plugin"
	"reflect"
	"regexp"
	"runtime"
	"sync"
)

const pluginSupported = true

func unsupportedReason() string { return "" }

type loadedPlugin struct {
	path string
	p    *goplugin.Plugin
}

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

func buildPlugin(name string, s *spec) (string, error) {
	src, err := generateSource(s)
	if err != nil {
		return "", err
	}

	flavour := ""
	if raceEnabled {
		flavour = "-race"
	}
	dir := filepath.Join(pluginDir(), name+"-"+sourceHash(src)+flavour)
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

	if ext := externalImports(s); len(ext) > 0 {
		if out, err := goCmd(dir, "mod", "tidy"); err != nil {
			return "", fmt.Errorf("go mod tidy failed for %v:\n%s", ext, out)
		}
	}

	args := []string{"build", "-buildmode=plugin"}
	if raceEnabled {
		args = append(args, "-race")
	}
	args = append(args, "-o", "plugin.so", "main.go")

	if out, err := goCmd(dir, args...); err != nil {
		return "", fmt.Errorf("go build -buildmode=plugin failed:\n%s", out)
	}
	return so, nil
}

func goCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func goMod(name string) string {
	v := runtime.Version()[len("go"):]
	if !goMinor.MatchString(v) {
		v = "1.21"
	}
	return fmt.Sprintf("module luascriptplugin/%s\n\ngo %s\n", name, v)
}
