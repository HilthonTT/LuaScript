//go:build !((linux || darwin || freebsd) && cgo)

package plugin

import (
	"runtime"
	"strings"
	"testing"
)

func TestStubReportsWhyItCannotLoadPlugins(t *testing.T) {
	if pluginSupported {
		t.Fatal("stub backend compiled in but pluginSupported is true")
	}

	reason := unsupportedReason()
	if !strings.Contains(reason, runtime.GOOS) {
		t.Errorf("reason %q does not name the platform (%s)", reason, runtime.GOOS)
	}

	s := &spec{
		Packages:  []pkg{{Name: "strings"}},
		Functions: []function{{Pkg: "strings", Name: "ToUpper", As: "ToUpper"}},
	}
	if _, err := buildPlugin("strutil", s); err == nil {
		t.Error("buildPlugin succeeded on an unsupported platform")
	} else if !strings.Contains(err.Error(), runtime.GOOS) {
		t.Errorf("buildPlugin error %q does not name the platform", err)
	}

	if _, err := openPlugin("whatever.so"); err == nil {
		t.Error("openPlugin succeeded on an unsupported platform")
	}
}
