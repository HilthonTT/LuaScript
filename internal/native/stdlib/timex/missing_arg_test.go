package timex_test

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/compiler"
	"github.com/hilthontt/luascript/v3/internal/compiler/parser"
	"github.com/hilthontt/luascript/v3/internal/native/stdlib/timex"
	"github.com/hilthontt/luascript/v3/internal/vm"
)

func TestFormatWithoutArgumentsIsArgumentError(t *testing.T) {
	chunks, err := compiler.CompileToInstructions(`--!nocheck
local time = require("time")
time.format()`, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v := vm.New()
	timex.RegisterTimePreload(v)
	e := v.Run(chunks[0])
	if e == nil || strings.Contains(e.Error(), "runtime error") || !strings.Contains(e.Error(), "bad argument #1") {
		t.Errorf("time.format() = %v, want a bad-argument error", e)
	}
}
