package ui

import "github.com/hilthontt/luascript/internal/vm"

func RegisterUIPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ui", uiLoader)
}
