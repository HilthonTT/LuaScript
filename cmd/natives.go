package main

import (
	"github.com/hilthontt/luascript/native/bit32"
	"github.com/hilthontt/luascript/native/compression"
	"github.com/hilthontt/luascript/native/crypto"
	"github.com/hilthontt/luascript/native/db"
	"github.com/hilthontt/luascript/native/debugx"
	"github.com/hilthontt/luascript/native/enumrt"
	httpNative "github.com/hilthontt/luascript/native/http"
	"github.com/hilthontt/luascript/native/httpserver"
	"github.com/hilthontt/luascript/native/iox"
	"github.com/hilthontt/luascript/native/json"
	"github.com/hilthontt/luascript/native/logx"
	"github.com/hilthontt/luascript/native/math"
	osNative "github.com/hilthontt/luascript/native/os"
	regexpNative "github.com/hilthontt/luascript/native/regexp"
	"github.com/hilthontt/luascript/native/sort"
	"github.com/hilthontt/luascript/native/std"
	"github.com/hilthontt/luascript/native/timex"
	"github.com/hilthontt/luascript/native/ui"
	"github.com/hilthontt/luascript/native/utf8x"
	"github.com/hilthontt/luascript/native/uuid"
	"github.com/hilthontt/luascript/vm"
)

// nativeRegistrars is the single source of truth for which native
// modules ship with the interpreter. Both the CLI path (cmd/main.go,
// via repl.AddPostInit) and the bundled-binary path (cmd/build.go's
// runBundled) walk this slice — adding a new module here is the only
// change needed to surface it in both contexts.
//
// Order is not load-bearing: each registrar only installs a
// package.preload entry; the loader runs on first `require`.
var nativeRegistrars = []func(*vm.VM){
	db.RegisterDBPreload,
	osNative.RegisterOSPreload,
	math.RegisterMathPreload,
	json.RegisterJSONPreload,
	httpNative.RegisterHttpPreload,
	httpserver.RegisterHTTPServerPreload,
	crypto.RegisterCryptoPreload,
	timex.RegisterTimePreload,
	regexpNative.RegisterRegexpPreload,
	uuid.RegisterUUIDPreload,
	sort.RegisterSortPreload,
	std.RegisterStdPreload,
	compression.RegisterCompressionPreload,
	bit32.RegisterBit32Preload,
	utf8x.RegisterUTF8Preload,
	iox.RegisterIOPreload,
	logx.RegisterLogPreload,
	debugx.RegisterDebugPreload,
	ui.RegisterUIPreload,
	// enumrt installs an internal global (__enum_freeze) the bytecode
	// generator calls when lowering `enum` declarations. Not a require()
	// target — placed in nativeRegistrars purely so it lands on both the
	// CLI VM (via repl.AddPostInit) and the bundled-binary VM (via
	// registerAllNatives) without a separate plumbing pass.
	enumrt.RegisterEnumRT,
}

// registerAllNatives applies each registrar to the given VM directly.
// Used by the bundled-binary code path where there is no REPL.
func registerAllNatives(v *vm.VM) {
	for _, r := range nativeRegistrars {
		r(v)
	}
}
