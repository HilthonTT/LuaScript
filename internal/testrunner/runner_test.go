package testrunner_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/testrunner"
)

// write drops a file into dir, creating parents, and returns its path.
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// runIn executes a run rooted at dir and returns the summary and the report.
// The bytecode cache is redirected per-test so a run cannot pick up a chunk
// another test compiled from identical source under a different name.
func runIn(t *testing.T, dir string, tune func(*testrunner.Options)) (testrunner.Summary, string) {
	t.Helper()
	t.Setenv("LUASCRIPT_CACHE_DIR", t.TempDir())

	var buf bytes.Buffer
	opts := testrunner.Options{Paths: []string{dir}, Out: &buf}
	if tune != nil {
		tune(&opts)
	}
	sum, err := testrunner.Run(opts)
	if err != nil {
		t.Fatalf("Run: %v\noutput:\n%s", err, buf.String())
	}
	return sum, buf.String()
}

const passingSuite = `
local t = require("test")
t.describe("group", function()
	t.test("one", function() t.assert_eq(1, 1) end)
	t.test("two", function() t.assert_true(true) end)
end)
t.skip("later")
`

const failingSuite = `
local t = require("test")
t.test("bad", function() t.assert_eq(1, 2) end)
t.test("good", function() end)
`

func TestRunReportsPassesAndFailures(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pass_test.lsc", passingSuite)
	write(t, dir, "fail_test.lsc", failingSuite)

	sum, out := runIn(t, dir, nil)

	if sum.Files != 2 {
		t.Errorf("Files = %d, want 2", sum.Files)
	}
	if sum.Passed != 3 || sum.Failed != 1 || sum.Skipped != 1 {
		t.Errorf("counts = (%d passed, %d failed, %d skipped), want (3, 1, 1)",
			sum.Passed, sum.Failed, sum.Skipped)
	}
	if sum.OK() {
		t.Error("OK() = true with a failing test")
	}
	if !strings.Contains(out, "FAIL") || !strings.Contains(out, "bad") {
		t.Errorf("report did not name the failure:\n%s", out)
	}
	// A quiet run reports failures only; passing tests stay out of the way.
	if strings.Contains(out, "group/one") {
		t.Errorf("quiet run listed a passing test:\n%s", out)
	}
}

func TestVerboseListsEveryTest(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pass_test.lsc", passingSuite)

	sum, out := runIn(t, dir, func(o *testrunner.Options) { o.Verbose = true })

	if !sum.OK() {
		t.Errorf("OK() = false for an all-passing run:\n%s", out)
	}
	for _, want := range []string{"group/one", "group/two", "later", "PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("verbose report missing %q:\n%s", want, out)
		}
	}
}

func TestFilterNarrowsTheRun(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pass_test.lsc", passingSuite)

	sum, out := runIn(t, dir, func(o *testrunner.Options) {
		o.Filter = "one"
		o.Verbose = true
	})

	if sum.Passed != 1 || sum.Skipped != 0 {
		t.Errorf("counts = (%d passed, %d skipped), want (1, 0)", sum.Passed, sum.Skipped)
	}
	if strings.Contains(out, "group/two") {
		t.Errorf("filter let an unmatched test through:\n%s", out)
	}
}

// TestFilterHidesFilesThatMatchedNothing: when the user has asked about a
// subset, every other file is noise.
func TestFilterHidesFilesThatMatchedNothing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pass_test.lsc", passingSuite)
	write(t, dir, "other_test.lsc", `
		local t = require("test")
		t.test("unrelated", function() end)
	`)

	_, out := runIn(t, dir, func(o *testrunner.Options) {
		o.Filter = "group/"
		o.Verbose = true
	})

	if strings.Contains(out, "other_test.lsc") {
		t.Errorf("report mentioned a file with no matching tests:\n%s", out)
	}
	if !strings.Contains(out, "pass_test.lsc") {
		t.Errorf("report dropped the file that did match:\n%s", out)
	}
}

func TestListDoesNotRunBodies(t *testing.T) {
	dir := t.TempDir()
	// A body that would fail loudly if it ever executed.
	write(t, dir, "list_test.lsc", `
		local t = require("test")
		t.test("never runs", function() error("body executed") end)
	`)

	sum, out := runIn(t, dir, func(o *testrunner.Options) { o.List = true })

	if sum.Failed != 0 {
		t.Errorf("Failed = %d, want 0 — bodies must not run under List", sum.Failed)
	}
	if !strings.Contains(out, "never runs") || !strings.Contains(out, "1 tests") {
		t.Errorf("list output unexpected:\n%s", out)
	}
}

func TestFailFastStopsTheRun(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a_test.lsc", failingSuite)
	write(t, dir, "b_test.lsc", passingSuite)

	sum, _ := runIn(t, dir, func(o *testrunner.Options) { o.FailFast = true })

	if sum.Files != 1 {
		t.Errorf("Files = %d, want 1 — failfast should not reach the second file", sum.Files)
	}
	if sum.Failed != 1 {
		t.Errorf("Failed = %d, want 1", sum.Failed)
	}
}

// TestCompileErrorIsAFileError: a file that will not compile is reported as a
// file error rather than crashing the run, and the remaining files still run.
func TestCompileErrorIsAFileError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken_test.lsc", `local t = require("test") t.test(`)
	write(t, dir, "ok_test.lsc", passingSuite)

	sum, out := runIn(t, dir, nil)

	if sum.FileErrors != 1 {
		t.Errorf("FileErrors = %d, want 1", sum.FileErrors)
	}
	if sum.Passed != 2 {
		t.Errorf("Passed = %d, want 2 — the healthy file should still run", sum.Passed)
	}
	if sum.OK() {
		t.Error("OK() = true despite a file error")
	}
	if !strings.Contains(out, "ERR") {
		t.Errorf("report did not flag the broken file:\n%s", out)
	}
}

// TestChunkErrorKeepsEarlierResults: an error in top-level code after some
// tests have run must not discard them.
func TestChunkErrorKeepsEarlierResults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "half_test.lsc", `
		local t = require("test")
		t.test("ran", function() end)
		error("top-level boom")
	`)

	sum, out := runIn(t, dir, nil)

	if sum.Passed != 1 {
		t.Errorf("Passed = %d, want 1", sum.Passed)
	}
	if sum.FileErrors != 1 {
		t.Errorf("FileErrors = %d, want 1", sum.FileErrors)
	}
	if !strings.Contains(out, "chunk error") || !strings.Contains(out, "top-level boom") {
		t.Errorf("report did not surface the chunk error:\n%s", out)
	}
}

// TestFilesAreIsolated: each file gets its own VM, so a global set by one is
// invisible to the next.
func TestFilesAreIsolated(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a_test.lsc", `
		local t = require("test")
		leaked = "from a"
		t.test("sets a global", function() t.assert_eq(leaked, "from a") end)
	`)
	write(t, dir, "b_test.lsc", `
		local t = require("test")
		t.test("does not see it", function() t.assert_nil(leaked) end)
	`)

	sum, out := runIn(t, dir, nil)
	if !sum.OK() {
		t.Errorf("globals leaked between files:\n%s", out)
	}
}

func TestDiscoverWalksDirectoriesAndSkipsModules(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "top_test.lsc", "")
	write(t, dir, filepath.Join("nested", "deep_test.lsc"), "")
	write(t, dir, "helper.lsc", "")                                        // no suffix
	write(t, dir, filepath.Join("lua_modules", "dep", "dep_test.lsc"), "") // dependency
	write(t, dir, filepath.Join(".hidden", "hidden_test.lsc"), "")         // tool metadata

	found, err := testrunner.Discover([]string{dir})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var names []string
	for _, f := range found {
		names = append(names, filepath.Base(f))
	}
	got := strings.Join(names, ",")
	if got != "deep_test.lsc,top_test.lsc" && got != "top_test.lsc,deep_test.lsc" {
		t.Errorf("Discover found %v, want only the two first-party files", names)
	}
}

// TestDiscoverTakesNamedFilesVerbatim: naming a file explicitly runs it even
// when it does not follow the suffix convention.
func TestDiscoverTakesNamedFilesVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "scratch.lsc", "")

	found, err := testrunner.Discover([]string{path})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 1 || filepath.Base(found[0]) != "scratch.lsc" {
		t.Errorf("Discover(%q) = %v, want the file itself", path, found)
	}
}

func TestDiscoverRejectsMissingPath(t *testing.T) {
	if _, err := testrunner.Discover([]string{filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Error("Discover accepted a path that does not exist")
	}
}

func TestRunWithNoTestFilesIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "notatest.lsc", "")

	_, err := testrunner.Run(testrunner.Options{Paths: []string{dir}, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("Run succeeded with no test files; want an error")
	}
	if !strings.Contains(err.Error(), testrunner.Suffix) {
		t.Errorf("error = %q, want it to name the expected suffix", err)
	}
}

// TestColorAutoIsOffForNonTerminals guards the common case: piping the report
// into a file or a CI log must not embed escape sequences.
func TestColorAutoIsOffForNonTerminals(t *testing.T) {
	if testrunner.ColorAuto(&bytes.Buffer{}) {
		t.Error("ColorAuto(buffer) = true, want false")
	}
}

func TestReportIsPlainWithoutColor(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "fail_test.lsc", failingSuite)

	_, out := runIn(t, dir, nil)
	if strings.Contains(out, "\033[") {
		t.Errorf("report contains ANSI escapes with Color off:\n%q", out)
	}
}
