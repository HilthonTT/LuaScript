package stats_test

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
	"github.com/hilthontt/luascript/v3/internal/native/datascience/stats"
	"github.com/hilthontt/luascript/v3/internal/vm"
)

// NaN slipped past `q < 0 || q > 1`, and int(NaN) then indexed the sorted
// slice at math.MinInt64 — a Go runtime panic instead of an argument error.
func TestQuantileRejectsNaN(t *testing.T) {
	for _, call := range []string{"stats.quantile({1, 2, 3}, 0/0)", "stats.percentile({1, 2, 3}, 0/0)"} {
		chunks, err := compiler.CompileToInstructions(`local stats = require("stats")
`+call, parser.NormalMode)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		v := vm.New()
		stats.RegisterStatsPreload(v)
		e := v.Run(chunks[0])
		if e == nil || strings.Contains(e.Error(), "runtime error") || !strings.Contains(e.Error(), "must be in") {
			t.Errorf("%s = %v, want a 'must be in' argument error", call, e)
		}
	}
}

func TestHistogramRejectsHugeBinCount(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`local stats = require("stats")
stats.histogram({1, 2, 3}, 10000000000000)`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v := vm.New()
	stats.RegisterStatsPreload(v)
	if e := v.Run(chunks[0]); e == nil || !strings.Contains(e.Error(), "bins must be between") {
		t.Errorf("stats.histogram with 1e13 bins = %v, want a bins-range error", e)
	}
}
