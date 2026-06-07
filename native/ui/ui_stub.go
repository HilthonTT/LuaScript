//go:build luascript_no_window

// Stub `ui` loader used when the `luascript_no_window` build tag is set.
// Drops the Fyne/OpenGL/cgo dep weight from the binary; `require("ui")`
// still succeeds at the package.preload stage and only panics when the
// script actually calls a constructor, matching how other optional
// drivers (native/db driver_postgres.go and friends) signal unavailability.
package ui

import "github.com/hilthontt/luascript/vm"

func uiLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	panic(vm.Errorf("ui: module unavailable (binary built with -tags luascript_no_window)"))
}
