package main

import (
	"os"

	"github.com/hilthontt/luascript/internal/native/stdlib/testx"
	"github.com/hilthontt/luascript/internal/vm"
)

// scriptTestTracker lets a plain `luascript foo_test.lsc` run (and a bundled
// binary) report test failures through the exit status, the way
// `luascript test` does. It re-installs the `test` module with a registry it
// keeps a handle on; the module itself never touches the exit code.
type scriptTestTracker struct {
	reg *testx.Registry
}

// install replaces the `test` preload on v with one whose results are
// tracked. It must run after the native registrars.
func (t *scriptTestTracker) install(v *vm.VM) {
	reg := testx.NewRegistry()
	reg.OnResult = testx.StandaloneReporter(os.Stdout)
	testx.Install(v, reg)
	t.reg = reg
}

// failed reports whether any test registered through `require("test")`
// in the most recently installed VM failed.
func (t *scriptTestTracker) failed() bool {
	return t.reg != nil && t.reg.Failed()
}
