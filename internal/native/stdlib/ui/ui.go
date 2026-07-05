// Package ui exposes a thin Lua binding over Fyne v2 widgets and
// windows. The Fyne-backed implementation (ui_fyne.go) is opt-in behind
// the `luascript_ui` build tag, since it drags in OpenGL/cgo and needs a
// C toolchain. By default ui_stub.go substitutes a no-op loader so a plain
// `go build ./cmd` stays pure-Go and dependency-light; `require("ui")`
// still resolves, only failing if a script actually constructs a widget.
// This file is the tag-agnostic entry point both code paths share.
package ui

import "github.com/hilthontt/luascript/internal/vm"

// RegisterUIPreload installs the `ui` module under package.preload.
// `require("ui")` invokes the tag-selected uiLoader on first use.
func RegisterUIPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ui", uiLoader)
}
