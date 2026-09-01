package bccache

import (
	"os"
	"path/filepath"
	"testing"
)

const testSrc = `
local function fib(n)
	if n < 2 then return n end
	return fib(n - 1) + fib(n - 2)
end
return fib(10)
`

func withTempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LUASCRIPT_CACHE_DIR", dir)
	t.Setenv("LUASCRIPT_NOCACHE", "")
	return dir
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.lscb"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestCompileCachedStoresAndHits(t *testing.T) {
	dir := withTempCache(t)

	first, err := CompileCached(testSrc)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	files := entries(t, dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 cache entry, got %d", len(files))
	}

	second, err := CompileCached(testSrc)
	if err != nil {
		t.Fatalf("cached compile: %v", err)
	}
	if len(second.Instructions) != len(first.Instructions) {
		t.Fatalf("cache hit produced %d instructions, fresh compile %d",
			len(second.Instructions), len(first.Instructions))
	}
	for i, w := range first.Instructions {
		g := second.Instructions[i]
		if g.Opcode != w.Opcode || g.A != w.A || g.B != w.B || g.StrA != w.StrA || g.BoxedAny != w.BoxedAny {
			t.Fatalf("instruction %d differs between fresh and cached compile:\nfresh:  %s\ncached: %s",
				i, w.Inspect(), g.Inspect())
		}
	}
}

func TestCompileCachedDistinguishesSources(t *testing.T) {
	dir := withTempCache(t)
	if _, err := CompileCached("return 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCached("return 2"); err != nil {
		t.Fatal(err)
	}
	if got := len(entries(t, dir)); got != 2 {
		t.Fatalf("expected 2 cache entries for 2 sources, got %d", got)
	}
}

func TestCompileCachedRecoversFromCorruptEntry(t *testing.T) {
	dir := withTempCache(t)
	if _, err := CompileCached(testSrc); err != nil {
		t.Fatal(err)
	}
	for _, f := range entries(t, dir) {
		if err := os.WriteFile(f, []byte("garbage"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	main, err := CompileCached(testSrc)
	if err != nil {
		t.Fatalf("corrupt entry was not recovered: %v", err)
	}
	if len(main.Instructions) == 0 {
		t.Fatal("recovered chunk is empty")
	}
}

func TestCompileCachedNeverCachesErrors(t *testing.T) {
	dir := withTempCache(t)
	if _, err := CompileCached("local = broken ("); err == nil {
		t.Fatal("expected a compile error")
	}
	if got := len(entries(t, dir)); got != 0 {
		t.Fatalf("a failed compile left %d cache entries", got)
	}
}

func TestNocacheDisables(t *testing.T) {
	dir := withTempCache(t)
	t.Setenv("LUASCRIPT_NOCACHE", "1")
	if _, err := CompileCached(testSrc); err != nil {
		t.Fatal(err)
	}
	if got := len(entries(t, dir)); got != 0 {
		t.Fatalf("LUASCRIPT_NOCACHE=1 still wrote %d cache entries", got)
	}
}
