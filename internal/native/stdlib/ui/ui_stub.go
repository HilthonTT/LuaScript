//go:build !luascript_ui

package ui

import "github.com/hilthontt/luascript/internal/vm"

func uiLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	panic(vm.Errorf("ui: module unavailable (rebuild with -tags luascript_ui to enable the Fyne-backed GUI)"))
}
