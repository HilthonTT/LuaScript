package pkgmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Install used to accept whatever commit the fetch produced and rewrite the
// lock to match, so the lock file pinned nothing.
func TestInstallRejectsCommitThatDoesNotMatchLock(t *testing.T) {
	f := &fakeFetcher{commits: map[string]string{"github.com/alice/router@pinnedcommit99": "attackercommit"}}
	env := newEnv(t, f)
	m := &Manifest{Dependencies: map[string]string{"router": "github.com/alice/router@v1.0.0"}}
	if err := m.Save(env.manifestPath()); err != nil {
		t.Fatal(err)
	}
	l := &Lock{Packages: map[string]LockEntry{"router": {Source: "github.com/alice/router", Commit: "pinnedcommit99"}}}
	if err := l.Save(env.lockPath()); err != nil {
		t.Fatal(err)
	}

	err := env.Install()
	if err == nil || !strings.Contains(err.Error(), "does not match locked commit") {
		t.Fatalf("Install = %v, want a lock-mismatch error", err)
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, ModulesDir, "router")); statErr == nil {
		t.Error("mismatched checkout was left in lua_modules")
	}
	back, _ := LoadLock(env.lockPath())
	if back.Packages["router"].Commit != "pinnedcommit99" {
		t.Errorf("lock rewritten to %q", back.Packages["router"].Commit)
	}
}

func TestIsCommitID(t *testing.T) {
	if !isCommitID(strings.Repeat("a1", 20)) || isCommitID("v1.0.0") || isCommitID(strings.Repeat("A", 40)) {
		t.Fatal("isCommitID misclassified")
	}
}
