package main

import (
	"github.com/hilthontt/luascript/v2/internal/native/datascience/classification"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/clustering"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/csv"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/dataframe"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/linalg"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/ml/luaml"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/ndarray"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/plot"
	"github.com/hilthontt/luascript/v2/internal/native/datascience/stats"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/bit32"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/compression"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/crypto"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/db"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/debugx"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/enumrt"
	httpNative "github.com/hilthontt/luascript/v2/internal/native/stdlib/http"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/httpserver"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/iox"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/json"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/logx"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/math"
	osNative "github.com/hilthontt/luascript/v2/internal/native/stdlib/os"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/queue"
	regexpNative "github.com/hilthontt/luascript/v2/internal/native/stdlib/regexp"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/sort"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/std"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/structrt"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/tablert"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/testx"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/timex"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/ui"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/utf8x"
	"github.com/hilthontt/luascript/v2/internal/native/stdlib/uuid"
	"github.com/hilthontt/luascript/v2/internal/plugin"
	"github.com/hilthontt/luascript/v2/internal/vm"
)

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
	plugin.RegisterPluginPreload,
	enumrt.RegisterEnumRT,
	structrt.RegisterStructRT,
	tablert.RegisterTableRT,
	promoteStandardGlobals,
}

var stdGlobalModules = []string{"os", "io", "utf8"}

func promoteStandardGlobals(v *vm.VM) {
	for _, name := range stdGlobalModules {
		vm.PromoteToGlobal(v, name)
	}
}

func registerAllNatives(v *vm.VM) {
	for _, r := range nativeRegistrars {
		r(v)
	}
}
