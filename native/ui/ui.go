// Package ui exposes a thin Lua binding over Fyne v2 widgets and
// windows. The real implementation lives in ui_fyne.go; ui_stub.go
// substitutes a no-op loader when the `luascript_no_window` build tag
// is set so the OpenGL/cgo dep weight can be dropped for headless
// builds. This file is the tag-agnostic entry point both code paths
// share.
package ui

import "github.com/hilthontt/luascript/vm"

// RegisterUIPreload installs the `ui` module under package.preload.
// `require("ui")` invokes the tag-selected uiLoader on first use.
func RegisterUIPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ui", uiLoader)
}
