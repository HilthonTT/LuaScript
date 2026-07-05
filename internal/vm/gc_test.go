package vm

import (
	"strings"
	"testing"
)

func TestCollectgarbageCollectReturnsZero(t *testing.T) {
	v := run(t, `r = collectgarbage("collect")`)
	assertGlobalEqual(t, v, "r", int64(0))
}

func TestCollectgarbageDefaultsToCollect(t *testing.T) {
	// No option arg defaults to "collect", returning 0.
	v := run(t, `r = collectgarbage()`)
	assertGlobalEqual(t, v, "r", int64(0))
}

func TestCollectgarbageCountReturnsNumber(t *testing.T) {
	v := run(t, `kb = collectgarbage("count")`)
	if got, ok := global(t, v, "kb").(float64); !ok || got <= 0 {
		t.Errorf(`collectgarbage("count") = %v (%T), want a positive float`, global(t, v, "kb"), global(t, v, "kb"))
	}
}

func TestCollectgarbageIsRunning(t *testing.T) {
	v := run(t, `r = collectgarbage("isrunning")`)
	if got, ok := global(t, v, "r").(bool); !ok || !got {
		t.Errorf(`collectgarbage("isrunning") = %v (%T), want true`, global(t, v, "r"), global(t, v, "r"))
	}
}

func TestCollectgarbageSetpauseReturnsPrevious(t *testing.T) {
	// setpause returns the previous GOGC; restart afterwards so the round-trip
	// doesn't leak an altered GC percent into sibling tests.
	v := run(t, `
		first = collectgarbage("setpause", 200)
		second = collectgarbage("setpause", 100)
	`)
	if got, ok := global(t, v, "second").(int64); !ok || got != 200 {
		t.Errorf(`second setpause returned %v (%T), want 200`, global(t, v, "second"), global(t, v, "second"))
	}
}

func TestCollectgarbageInvalidOption(t *testing.T) {
	msg := runErr(t, `collectgarbage("bogus")`)
	if want := "invalid option 'bogus'"; !strings.Contains(msg, want) {
		t.Errorf("error = %q, want it to contain %q", msg, want)
	}
}
