package typecheck

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// `L<L<T>>` inside L's own body branches at every expansion level; the depth
// cap alone still allowed ~2^64 expansions and the checker hung.
func TestBranchingRecursiveGenericTerminates(t *testing.T) {
	done := make(chan []TypeError, 1)
	go func() {
		done <- runCheckWith(t, `--!strict
type L<T> = { v: T, n: L<L<T>>? }
local x: L<number> = { v = 1 }
local y: L<string> = x`, Options{Strict: true})
	}()
	select {
	case errs := <-done:
		n := 0
		for _, e := range errs {
			if strings.Contains(e.Format(), "expands recursively") {
				n++
			}
		}
		if n == 0 {
			t.Fatalf("want a recursive-generic error, got %v", errs)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("checker did not terminate on a branching recursive generic")
	}
}

// Narrowing through a long left-deep `and` chain re-walked the whole chain at
// every level (quadratic); it now reuses the chain's refinement, and must
// still narrow every operand.
func TestLongAndChainStillNarrows(t *testing.T) {
	var b strings.Builder
	const vars, terms = 40, 3000
	for i := range vars {
		fmt.Fprintf(&b, "local v%d: number? = nil\n", i)
	}
	b.WriteString("local ok = ")
	for i := range terms {
		if i > 0 {
			b.WriteString(" and ")
		}
		fmt.Fprintf(&b, "v%d", i%vars)
	}
	b.WriteString(" and v0 + v39 > 0\n")
	expectOK(t, b.String())
}
