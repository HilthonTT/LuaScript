package iox_test

import (
	"strings"
	"testing"
)

// stdout/stderr and io.output handles carry no reader; iterating their lines
// used to dereference a nil *bufio.Reader.
func TestLinesOnUnreadableHandleIsLuaError(t *testing.T) {
	v := runIO(t, `
		local io = require("io")
		local ok1, e1 = pcall(function() for _ in io.stdout:lines() do end end)
		io.input(io.stdout)
		local ok2, e2 = pcall(function() for _ in io.lines() do end end)
		err1, err2 = tostring(e1), tostring(e2)
		okAny = ok1 or ok2
	`)
	if v.Globals.Get("okAny") != false {
		t.Fatalf("lines on an unreadable handle succeeded")
	}
	for _, name := range []string{"err1", "err2"} {
		msg, _ := v.Globals.Get(name).(string)
		if strings.Contains(msg, "runtime error") || !strings.Contains(msg, "not readable") {
			t.Errorf("%s = %q, want a 'not readable' Lua error", name, msg)
		}
	}
}
