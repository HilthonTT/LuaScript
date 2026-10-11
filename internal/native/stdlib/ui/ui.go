package ui

import "github.com/hilthontt/luascript/v3/internal/vm"

func RegisterUIPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ui", uiLoader)
}
