package ui

import "github.com/hilthontt/luascript/v2/internal/vm"

func RegisterUIPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ui", uiLoader)
}
