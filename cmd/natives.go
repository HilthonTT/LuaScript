package main

import (
	"github.com/hilthontt/sakura-lang/native/crypto"
	"github.com/hilthontt/sakura-lang/native/db"
	httpNative "github.com/hilthontt/sakura-lang/native/http"
	"github.com/hilthontt/sakura-lang/native/httpserver"
	"github.com/hilthontt/sakura-lang/native/json"
	"github.com/hilthontt/sakura-lang/native/math"
	osNative "github.com/hilthontt/sakura-lang/native/os"
	regexpNative "github.com/hilthontt/sakura-lang/native/regexp"
	"github.com/hilthontt/sakura-lang/native/sort"
	"github.com/hilthontt/sakura-lang/native/timex"
	"github.com/hilthontt/sakura-lang/native/uuid"
	"github.com/hilthontt/sakura-lang/vm"
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
}

// registerAllNatives applies each registrar to the given VM directly.
// Used by the bundled-binary code path where there is no REPL.
func registerAllNatives(v *vm.VM) {
	for _, r := range nativeRegistrars {
		r(v)
	}
}
