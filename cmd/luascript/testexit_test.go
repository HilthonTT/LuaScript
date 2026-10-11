package main

import (
	"os"
	"path/filepath"
	"testing"
)

const failingTestScript = `local test = require("test")
test.it("passes", function() test.assert_eq(1, 1) end)
test.it("fails", function() test.assert_eq(1, 2) end)
`

const passingTestScript = `local test = require("test")
test.it("passes", function() test.assert_eq(1, 1) end)
`

func writeScript(t *testing.T, name, src string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDirectRunOfFailingTestFileExitsNonZero(t *testing.T) {
	if code := run([]string{writeScript(t, "x_test.lsc", failingTestScript)}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if code := run([]string{"-time", writeScript(t, "y_test.lsc", failingTestScript)}); code != 1 {
		t.Fatalf("-time exit code = %d, want 1", code)
	}
}

func TestDirectRunOfPassingTestFileExitsZero(t *testing.T) {
	if code := run([]string{writeScript(t, "ok_test.lsc", passingTestScript)}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestDirectRunRuntimeErrorExitsNonZero(t *testing.T) {
	if code := run([]string{writeScript(t, "boom.lsc", `error("boom")`)}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestBundledFailingTestsExitNonZero(t *testing.T) {
	if code := runBundled(failingTestScript); code != 1 {
		t.Fatalf("bundled exit code = %d, want 1", code)
	}
	if code := runBundled(passingTestScript); code != 0 {
		t.Fatalf("bundled exit code = %d, want 0", code)
	}
}
