//go:build !luascript_ui

// Stub `ui` loader used by default (the Fyne implementation is opt-in via
// the `luascript_ui` build tag). Keeps the interpreter free of the
// Fyne/OpenGL/cgo dependency — so a plain `go run ./cmd` needs no C
// toolchain. `require("ui")` still succeeds at the package.preload stage
// and only panics when the script actually calls a constructor, matching
// how other optional drivers (native/db driver_postgres.go and friends)
// signal unavailability.
package ui

import "github.com/hilthontt/luascript/vm"

func uiLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	panic(vm.Errorf("ui: module unavailable (rebuild with -tags luascript_ui to enable the Fyne-backed GUI)"))
}
