package ndarray

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hilthontt/luascript/v3/internal/vm"
)

// arange used to append until the process ran out of memory when the step
// could not advance start (1e17 + 1 == 1e17) or stop was infinite.
func TestArangeRejectsUnboundedRanges(t *testing.T) {
	cases := map[string][]vm.Value{
		"step too small": {1e17, 1e17 + 100, 1.0},
		"infinite stop":  {0.0, math.Inf(1), 1.0},
		"too many":       {0.0, 1e12, 1.0},
		"neg infinite":   {0.0, math.Inf(-1), -1.0},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			done := make(chan any, 1)
			go func() {
				defer func() { done <- recover() }()
				arange(args)
			}()
			select {
			case r := <-done:
				e, ok := r.(vm.LuaError)
				if !ok || !strings.Contains(string(e), "ndarray.arange") {
					t.Fatalf("arange(%v) = %v, want an ndarray.arange error", args, r)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("arange(%v) did not return", args)
			}
		})
	}
}

func TestArangeStillWorks(t *testing.T) {
	eqData(t, arange([]vm.Value{int64(4)}), 0, 1, 2, 3)
	eqData(t, arange([]vm.Value{5.0, 1.0, -2.0}), 5, 3)
	eqData(t, arange([]vm.Value{3.0, 1.0}))
}
