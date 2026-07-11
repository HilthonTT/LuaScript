//go:build !((linux || darwin || freebsd) && cgo)

// Stub plugin backend for platforms where Go cannot load plugins at all.
//
// Go's `plugin` package is implemented only on linux/darwin/freebsd, and only
// with cgo enabled; on Windows plugin.Open unconditionally returns
// "plugin: not implemented". Rather than fail to compile — or, worse, compile
// and then die with that bare message at runtime — this file provides the same
// symbols as backend_native.go and reports the reason.
//
// The module itself still loads, so `require("plugin")` succeeds everywhere
// and a script can check `plugin.supported` before committing to a plugin path.
package plugin

import (
	"fmt"
	"reflect"
	"runtime"
)

const pluginSupported = false

func unsupportedReason() string {
	why := "requires linux, darwin or freebsd"
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" {
		// Right OS, so it must be the other half of the constraint.
		why = "requires cgo, which is disabled in this build (set CGO_ENABLED=1 and rebuild)"
	}
	return fmt.Sprintf("plugin: Go plugins are not supported on %s/%s — %s",
		runtime.GOOS, runtime.GOARCH, why)
}

func errUnsupported() error { return fmt.Errorf("%s", unsupportedReason()) }

type loadedPlugin struct{}

func openPlugin(string) (*loadedPlugin, error) { return nil, errUnsupported() }

func buildPlugin(string, *spec) (string, error) { return "", errUnsupported() }

func (lp *loadedPlugin) lookup(string) (reflect.Value, error) {
	return reflect.Value{}, errUnsupported()
}
