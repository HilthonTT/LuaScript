package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule drops src into a fresh file in dir and returns the absolute
// path, with backslashes normalised to forward slashes so the path can be
// embedded directly in a Lua string literal on Windows.
func writeModule(t *testing.T, dir, name, src string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return filepath.ToSlash(p)
}

// setupModuleDir creates a temp dir, writes a module to it, and returns
// (forwardslash dirpath, file path) for use in test source strings.
func setupModuleDir(t *testing.T, name, src string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	mp := writeModule(t, dir, name, src)
	return filepath.ToSlash(dir), mp
}

// ---------------------------------------------------------------------------
// require — basic load + cache
// ---------------------------------------------------------------------------

func TestRequireLoadsModuleAndReturnsValue(t *testing.T) {
	dir, _ := setupModuleDir(t, "greet.sakura", `return {hello = "hi"}`)
	src := fmt.Sprintf(`
		package.path = "%s/?.sakura"
		m = require("greet")
		r = m.hello
	`, dir)
	v := run(t, src)
	assertGlobalEqual(t, v, "r", "hi")
}

func TestRequireCachesModuleSecondCallNoReExec(t *testing.T) {
	dir := t.TempDir()
	dirSlash := filepath.ToSlash(dir)
	writeModule(t, dir, "counter.sakura", `
		_G.counterRuns = (_G.counterRuns or 0) + 1
		return _G.counterRuns
	`)
	src := fmt.Sprintf(`
		package.path = "%s/?.sakura"
		_G = _ENV or {} -- no _ENV in this VM; counter via package.loaded check below
		first = require("counter")
		second = require("counter")
	`, dirSlash)
	// Our VM has no _ENV; rewrite the test using a global the module mutates directly.
	src = fmt.Sprintf(`
		package.path = "%s/?.sakura"
		first = require("counter")
		second = require("counter")
		same = (first == second)
	`, dirSlash)
	// And replace the counter file with one that uses globals directly.
	writeModule(t, dir, "counter.sakura", `
		runs = (runs or 0) + 1
		return runs
	`)
	v := run(t, src)
	// The module ran exactly once; second require returned the cached value.
	assertGlobalEqual(t, v, "first", int64(1))
	assertGlobalEqual(t, v, "second", int64(1))
	assertGlobalEqual(t, v, "same", true)
	assertGlobalEqual(t, v, "runs", int64(1))
}

// ---------------------------------------------------------------------------
// dotted module names → / substitution
// ---------------------------------------------------------------------------

func TestRequireResolvesDottedNameToSubdirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeModule(t, subdir, "time.sakura", `return "from src/time"`)
	dirSlash := filepath.ToSlash(dir)
	src := fmt.Sprintf(`
		package.path = "%s/?.sakura"
		t = require("src.time")
	`, dirSlash)
	v := run(t, src)
	assertGlobalEqual(t, v, "t", "from src/time")
}

// ---------------------------------------------------------------------------
// .sakura wins over .lua when both exist
// ---------------------------------------------------------------------------

func TestRequirePrefersSakuraOverLua(t *testing.T) {
	dir := t.TempDir()
	dirSlash := filepath.ToSlash(dir)
	writeModule(t, dir, "shared.sakura", `return "sakura-version"`)
	writeModule(t, dir, "shared.lua", `return "lua-version"`)
	src := fmt.Sprintf(`
		package.path = "%s/?.sakura;%s/?.lua"
		x = require("shared")
	`, dirSlash, dirSlash)
	v := run(t, src)
	assertGlobalEqual(t, v, "x", "sakura-version")
}

// ---------------------------------------------------------------------------
// missing module
// ---------------------------------------------------------------------------

func TestRequireMissingModuleErrors(t *testing.T) {
	src := `
		package.path = "/nonexistent/?.sakura"
		m = require("nothing.here")
	`
	msg := runErr(t, src)
	if !strings.Contains(msg, "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", msg)
	}
	if !strings.Contains(msg, "nothing.here") {
		t.Errorf("error = %q, want the module name in the message", msg)
	}
}

// ---------------------------------------------------------------------------
// preload table — host-registered loaders
// ---------------------------------------------------------------------------

func TestPreloadHookRunsOnce(t *testing.T) {
	src := `
		runs = 0
		package.preload["fake"] = function(name)
			runs = runs + 1
			return {answer = 42, who = name}
		end
		a = require("fake")
		b = require("fake")
		v = a.answer
		who = a.who
		same = (a == b)
	`
	v := run(t, src)
	assertGlobalEqual(t, v, "runs", int64(1))
	assertGlobalEqual(t, v, "v", int64(42))
	assertGlobalEqual(t, v, "who", "fake")
	assertGlobalEqual(t, v, "same", true)
}

// ---------------------------------------------------------------------------
// package.searchpath
// ---------------------------------------------------------------------------

func TestSearchpathFindsExistingFile(t *testing.T) {
	dir, _ := setupModuleDir(t, "lib.sakura", `return 1`)
	src := fmt.Sprintf(`
		fpath = package.searchpath("lib", "%s/?.sakura;%s/?.lua")
	`, dir, dir)
	v := run(t, src)
	got, ok := global(t, v, "fpath").(string)
	if !ok || !strings.HasSuffix(filepath.ToSlash(got), "/lib.sakura") {
		t.Errorf("searchpath = %v, want path ending in /lib.sakura", got)
	}
}

func TestSearchpathReturnsNilAndMessageOnMiss(t *testing.T) {
	src := `
		fpath, msg = package.searchpath("missing", "/nope/?.sakura")
	`
	v := run(t, src)
	assertGlobalEqual(t, v, "fpath", nil)
	got, ok := global(t, v, "msg").(string)
	if !ok || !strings.Contains(got, "no file") {
		t.Errorf("msg = %v, want a string mentioning 'no file'", got)
	}
}

// ---------------------------------------------------------------------------
// loadfile / dofile / load
// ---------------------------------------------------------------------------

func TestLoadfileReturnsCallableChunk(t *testing.T) {
	_, fpath := setupModuleDir(t, "x.sakura", `return 7`)
	src := fmt.Sprintf(`
		fn = loadfile("%s")
		r = fn()
	`, fpath)
	v := run(t, src)
	assertGlobalEqual(t, v, "r", int64(7))
}

func TestLoadfileReturnsErrOnMissing(t *testing.T) {
	src := `
		fn, err = loadfile("/no/such/file.sakura")
	`
	v := run(t, src)
	assertGlobalEqual(t, v, "fn", nil)
	got, ok := global(t, v, "err").(string)
	if !ok || got == "" {
		t.Errorf("err = %v, want a non-empty error string", got)
	}
}

func TestDofileRunsImmediately(t *testing.T) {
	_, fpath := setupModuleDir(t, "y.sakura", `return 99`)
	src := fmt.Sprintf(`
		r = dofile("%s")
	`, fpath)
	v := run(t, src)
	assertGlobalEqual(t, v, "r", int64(99))
}

func TestLoadCompilesString(t *testing.T) {
	src := `
		fn = load("return 3 * 14")
		r = fn()
	`
	v := run(t, src)
	assertGlobalEqual(t, v, "r", int64(42))
}

// ---------------------------------------------------------------------------
// Module receives modname + path as ...
// ---------------------------------------------------------------------------

func TestModuleReceivesNameAndPathAsVarargs(t *testing.T) {
	dir, _ := setupModuleDir(t, "hello.sakura", `
		local n = ...
		return n
	`)
	src := fmt.Sprintf(`
		package.path = "%s/?.sakura"
		got = require("hello")
	`, dir)
	v := run(t, src)
	assertGlobalEqual(t, v, "got", "hello")
}
