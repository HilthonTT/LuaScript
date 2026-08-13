package main

import (
	"github.com/hilthontt/luascript/internal/native/datascience/classification"
	"github.com/hilthontt/luascript/internal/native/datascience/clustering"
	"github.com/hilthontt/luascript/internal/native/datascience/csv"
	"github.com/hilthontt/luascript/internal/native/datascience/dataframe"
	"github.com/hilthontt/luascript/internal/native/datascience/linalg"
	"github.com/hilthontt/luascript/internal/native/datascience/ml/luaml"
	"github.com/hilthontt/luascript/internal/native/datascience/ndarray"
	"github.com/hilthontt/luascript/internal/native/datascience/plot"
	"github.com/hilthontt/luascript/internal/native/datascience/stats"
	"github.com/hilthontt/luascript/internal/native/stdlib/bit32"
	"github.com/hilthontt/luascript/internal/native/stdlib/compression"
	"github.com/hilthontt/luascript/internal/native/stdlib/crypto"
	"github.com/hilthontt/luascript/internal/native/stdlib/db"
	"github.com/hilthontt/luascript/internal/native/stdlib/debugx"
	"github.com/hilthontt/luascript/internal/native/stdlib/enumrt"
	httpNative "github.com/hilthontt/luascript/internal/native/stdlib/http"
	"github.com/hilthontt/luascript/internal/native/stdlib/httpserver"
	"github.com/hilthontt/luascript/internal/native/stdlib/iox"
	"github.com/hilthontt/luascript/internal/native/stdlib/json"
	"github.com/hilthontt/luascript/internal/native/stdlib/logx"
	"github.com/hilthontt/luascript/internal/native/stdlib/math"
	osNative "github.com/hilthontt/luascript/internal/native/stdlib/os"
	"github.com/hilthontt/luascript/internal/native/stdlib/queue"
	regexpNative "github.com/hilthontt/luascript/internal/native/stdlib/regexp"
	"github.com/hilthontt/luascript/internal/native/stdlib/sort"
	"github.com/hilthontt/luascript/internal/native/stdlib/std"
	"github.com/hilthontt/luascript/internal/native/stdlib/structrt"
	"github.com/hilthontt/luascript/internal/native/stdlib/testx"
	"github.com/hilthontt/luascript/internal/native/stdlib/timex"
	"github.com/hilthontt/luascript/internal/native/stdlib/ui"
	"github.com/hilthontt/luascript/internal/native/stdlib/utf8x"
	"github.com/hilthontt/luascript/internal/native/stdlib/uuid"
	"github.com/hilthontt/luascript/internal/plugin"
	"github.com/hilthontt/luascript/internal/vm"
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
	queue.RegisterQueuePreload,
	compression.RegisterCompressionPreload,
	// test is registered like any other module so `require("test")` resolves
	// in a plain script run too. `luascript test` installs its own registry
	// over this one after walking the list — see internal/testrunner.
	testx.RegisterTestPreload,
	bit32.RegisterBit32Preload,
	utf8x.RegisterUTF8Preload,
	iox.RegisterIOPreload,
	logx.RegisterLogPreload,
	debugx.RegisterDebugPreload,
	ui.RegisterUIPreload,
	clustering.RegisterClusteringPreload,
	classification.RegisterClassificationPreload,
	stats.RegisterStatsPreload,
	linalg.RegisterLinalgPreload,
	csv.RegisterCSVPreload,
	dataframe.RegisterDataFramePreload,
	ndarray.RegisterNDArrayPreload,
	plot.RegisterPlotPreload,
	luaml.RegisterMLPreload,
	// plugin loads Go libraries dynamically (go build -buildmode=plugin +
	// reflection). Registered on every platform so `require("plugin")` always
	// resolves; on platforms without Go plugin support the module loads with
	// `supported = false` and generate/open raise. See internal/plugin.
	plugin.RegisterPluginPreload,
	// enumrt installs an internal global (__enum_freeze) the bytecode
	// generator calls when lowering `enum` declarations. Not a require()
	// target — placed in nativeRegistrars purely so it lands on both the
	// CLI VM (via repl.AddPostInit) and the bundled-binary VM (via
	// registerAllNatives) without a separate plumbing pass.
	enumrt.RegisterEnumRT,
	// structrt installs __struct_define, the constructor factory the
	// bytecode generator calls when lowering a `struct` declaration. Like
	// enumrt it is an internal emit target, not a require() module.
	structrt.RegisterStructRT,
	// Must stay last: it binds the modules registered above to globals, so
	// every preload entry it looks for has to already exist. Hooks run in
	// slice order on both the CLI and bundled paths, so appending here is
	// enough to guarantee that.
	promoteStandardGlobals,
}

// stdGlobalModules are the modules Lua 5.4 exposes as globals rather than as
// require() targets. This runtime implements all three natively — internal/vm
// cannot import internal/native — so without this step a script had to open
// with `local io = require("io")` before any of the reference manual's io.open
// examples would run.
//
// Deliberately short: promotion costs an eager load of each module at VM
// startup, and only these three are part of the standard global namespace.
// Everything else stays behind require, where it is paid for on first use.
var stdGlobalModules = []string{"os", "io", "utf8"}

func promoteStandardGlobals(v *vm.VM) {
	for _, name := range stdGlobalModules {
		vm.PromoteToGlobal(v, name)
	}
}

// registerAllNatives applies each registrar to the given VM directly.
// Used by the bundled-binary code path where there is no REPL.
func registerAllNatives(v *vm.VM) {
	for _, r := range nativeRegistrars {
		r(v)
	}
}
