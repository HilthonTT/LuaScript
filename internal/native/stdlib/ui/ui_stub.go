//go:build !luascript_ui

package ui

import "github.com/hilthontt/luascript/internal/vm"

// stubFunctions mirrors the module surface of the Fyne build (ui_fyne.go), so
// require("ui") resolves in every build and a script can feature-test with
// ui.supported. Only calling into the GUI raises.
var stubFunctions = []string{
	"new_window", "new_label", "new_button", "new_entry",
	"new_vbox", "new_hbox", "run", "quit",
}

func uiLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 2)
	mod.Set("VERSION", "0.1.0")
	mod.Set("supported", false)

	methods := vm.NewTable(0, len(stubFunctions))
	for _, name := range stubFunctions {
		site := "ui." + name
		methods.Set(name, &vm.GoFunc{Name: "ui:" + name, Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
			panic(vm.Errorf("%s: this luascript was built without -tags luascript_ui, so the Fyne-backed GUI is unavailable (check ui.supported first)", site))
		}})
	}

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	mod.SetMetatable(mt)
	return []vm.Value{mod}
}
