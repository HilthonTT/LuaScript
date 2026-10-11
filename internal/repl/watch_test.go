package repl

import (
	"os"
	"path/filepath"
	"testing"
)

// safeRunFile is what -watch calls on every save. A compile or runtime error
// must be reported and returned from, never os.Exit — otherwise the watcher
// dies on the first mistake. (If it exited, this test binary would too.)
func TestSafeRunFileSurvivesErrors(t *testing.T) {
	dir := t.TempDir()
	r := &REPL{}
	for name, src := range map[string]string{
		"runtime.lsc": `error("boom")`,
		"compile.lsc": `local = = 1`,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		r.safeRunFile(p)
		if err := r.ExecFile(p); err == nil {
			t.Fatalf("%s: ExecFile returned nil, want an error", name)
		}
	}
	r.safeRunFile(filepath.Join(dir, "missing.lsc"))
}
