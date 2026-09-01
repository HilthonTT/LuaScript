//go:build !((linux || darwin || freebsd) && cgo)

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
