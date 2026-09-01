package pkgmanager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		in         string
		wantSource string
		wantRef    string
		wantName   string
		wantErr    bool
	}{
		{"github.com/alice/router", "github.com/alice/router", "", "router", false},
		{"github.com/alice/router@v1.2.0", "github.com/alice/router", "v1.2.0", "router", false},
		{"gitlab.com/team/lib@main", "gitlab.com/team/lib", "main", "lib", false},
		{"host/a/b/c@deadbeef", "host/a/b/c", "deadbeef", "c", false},
		{"github.com/alice/thing.git", "github.com/alice/thing.git", "", "thing", false},
		{"", "", "", "", true},
		{"https://github.com/alice/router", "", "", "", true},
		{"git@github.com:alice/router", "", "", "", true},
		{"router", "", "", "", true},
	}
	for _, tc := range tests {
		got, err := ParseSpec(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSpec(%q): expected error, got %+v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSpec(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got.Source != tc.wantSource || got.Ref != tc.wantRef {
			t.Errorf("ParseSpec(%q) = {%q, %q}, want {%q, %q}",
				tc.in, got.Source, got.Ref, tc.wantSource, tc.wantRef)
		}
		if name := got.DefaultName(); name != tc.wantName {
			t.Errorf("ParseSpec(%q).DefaultName() = %q, want %q", tc.in, name, tc.wantName)
		}
	}
}

func TestCloneURL(t *testing.T) {
	s, _ := ParseSpec("github.com/alice/router@v1")
	if got, want := s.CloneURL(), "https://github.com/alice/router.git"; got != want {
		t.Errorf("CloneURL() = %q, want %q", got, want)
	}
	if got, want := s.String(), "github.com/alice/router@v1"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestValidName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "../escape", "x..y"} {
		if err := validName(bad); err == nil {
			t.Errorf("validName(%q): expected error", bad)
		}
	}
	for _, ok := range []string{"router", "json_ext", "my-pkg", "a"} {
		if err := validName(ok); err != nil {
			t.Errorf("validName(%q): unexpected error: %v", ok, err)
		}
	}
}

type fakeFetcher struct {
	calls   []Spec
	commits map[string]string
}

func (f *fakeFetcher) Fetch(spec Spec, destDir string) (string, error) {
	f.calls = append(f.calls, spec)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(destDir, "init.lsc"), []byte("return {}\n"), 0o644); err != nil {
		return "", err
	}
	commit := "c0ffee1234567890"
	if f.commits != nil {
		if c, ok := f.commits[spec.String()]; ok {
			commit = c
		}
	}
	return commit, nil
}

func newEnv(t *testing.T, f Fetcher) *Env {
	t.Helper()
	return &Env{Root: t.TempDir(), Fetcher: f}
}

func TestAddWritesManifestLockAndModule(t *testing.T) {
	f := &fakeFetcher{commits: map[string]string{"github.com/alice/router@v1.2.0": "abc123def456"}}
	env := newEnv(t, f)

	if err := env.Add("github.com/alice/router@v1.2.0", ""); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := os.Stat(filepath.Join(env.Root, ModulesDir, "router", "init.lsc")); err != nil {
		t.Errorf("expected installed module file: %v", err)
	}

	m, found, err := LoadManifest(env.manifestPath())
	if err != nil || !found {
		t.Fatalf("LoadManifest: found=%v err=%v", found, err)
	}
	if got := m.Dependencies["router"]; got != "github.com/alice/router@v1.2.0" {
		t.Errorf("manifest dep = %q", got)
	}

	l, err := LoadLock(env.lockPath())
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	entry := l.Packages["router"]
	if entry.Commit != "abc123def456" || entry.URL != "https://github.com/alice/router.git" {
		t.Errorf("lock entry = %+v", entry)
	}
}

func TestAddWithExplicitName(t *testing.T) {
	env := newEnv(t, &fakeFetcher{})
	if err := env.Add("github.com/bob/json_ext@v0.3.0", "json2"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, ModulesDir, "json2", "init.lsc")); err != nil {
		t.Errorf("expected module under explicit name: %v", err)
	}
	m, _, _ := LoadManifest(env.manifestPath())
	if _, ok := m.Dependencies["json2"]; !ok {
		t.Errorf("manifest missing json2: %+v", m.Dependencies)
	}
}

func TestInstallSkipsExistingAndFetchesLockedCommit(t *testing.T) {
	f := &fakeFetcher{}
	env := newEnv(t, f)

	m := &Manifest{
		Name:    "proj",
		Version: "0.1.0",
		Dependencies: map[string]string{
			"router": "github.com/alice/router@v1.0.0",
			"lib":    "gitlab.com/team/lib@main",
		},
	}
	if err := m.Save(env.manifestPath()); err != nil {
		t.Fatal(err)
	}
	l := &Lock{Packages: map[string]LockEntry{
		"router": {Source: "github.com/alice/router", Ref: "v1.0.0", URL: "https://github.com/alice/router.git", Commit: "pinnedcommit99"},
	}}
	if err := l.Save(env.lockPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(env.Root, ModulesDir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := env.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(f.calls) != 1 {
		t.Fatalf("expected 1 fetch, got %d: %+v", len(f.calls), f.calls)
	}
	if f.calls[0].Ref != "pinnedcommit99" {
		t.Errorf("expected fetch of locked commit, got ref %q", f.calls[0].Ref)
	}
}

func TestInstallWithoutManifestErrors(t *testing.T) {
	env := newEnv(t, &fakeFetcher{})
	if err := env.Install(); err == nil {
		t.Errorf("expected error installing with no manifest")
	}
}

func TestRemoveDeletesModuleAndEntries(t *testing.T) {
	env := newEnv(t, &fakeFetcher{})
	if err := env.Add("github.com/alice/router@v1", ""); err != nil {
		t.Fatal(err)
	}
	if err := env.Remove("router"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, ModulesDir, "router")); !os.IsNotExist(err) {
		t.Errorf("module dir should be gone, stat err = %v", err)
	}
	m, _, _ := LoadManifest(env.manifestPath())
	if _, ok := m.Dependencies["router"]; ok {
		t.Errorf("manifest should not list router")
	}
	l, _ := LoadLock(env.lockPath())
	if _, ok := l.Packages["router"]; ok {
		t.Errorf("lock should not list router")
	}
}
