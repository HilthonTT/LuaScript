package plot

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/vm"
)

func TestHistogramRejectsHugeBinCount(t *testing.T) {
	vals := vm.NewTable(3, 0)
	for _, x := range []int64{1, 2, 3} {
		vals.Append(x)
	}
	defer func() {
		e, ok := recover().(vm.LuaError)
		if !ok || !strings.Contains(string(e), "bins must be between") {
			t.Fatalf("got %v, want a bins-range error", e)
		}
	}()
	addHistogram(newFigure(), "plot.histogram", []vm.Value{vals, int64(1e13)}, 1)
}
