// Package pkgmanager implements luascript's git/URL-based package manager:
// the `luascript pkg add/install/remove` subcommands. It is deliberately
// server-less — a package is just a git repository (or any URL git can
// clone). Dependencies are recorded in a JSON manifest and pinned to exact
// commits in a JSON lockfile so installs are reproducible.
//
// The runtime side needs no cooperation: packages are cloned into
// `./lua_modules/<name>`, which `vm/loader.go` already searches via
// package.path. Installing a package and `require`-ing it are fully
// decoupled.
//
// JSON (not a `.lsc` table) is used for both files because the tool must
// round-trip them — read, mutate one dependency, write back — and hand-
// emitting Lua table literals from Go would not preserve formatting or
// comments anyway. Humans edit the manifest; the tool owns the lockfile.
package pkgmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// ManifestName is the per-project dependency manifest, human-editable.
const ManifestName = "luascript.json"

// LockName pins each resolved dependency to an exact commit.
const LockName = "luascript-lock.json"

// ModulesDir is the install root. It matches the `./lua_modules/...` entries
// added to baseSearchPath in vm/loader.go.
const ModulesDir = "lua_modules"

// Manifest is the parsed `luascript.json`. Dependencies maps a local package
// name (the name used in `require`) to a source spec of the form
// `host/path[@ref]` — e.g. "github.com/alice/router@v1.2.0".
type Manifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

// LoadManifest reads and parses the manifest at path. A missing file is not
// an error: it returns an empty manifest so `pkg add` can bootstrap a project
// that has none yet. The caller distinguishes "absent" via the second return.
func LoadManifest(path string) (*Manifest, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Manifest{Dependencies: map[string]string{}}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	if m.Dependencies == nil {
		m.Dependencies = map[string]string{}
	}
	return &m, true, nil
}

// Save writes the manifest as indented JSON with a trailing newline. Map keys
// serialize in sorted order (encoding/json guarantees this), so the file is
// stable across runs and diff-friendly.
func (m *Manifest) Save(path string) error {
	return writeJSON(path, m)
}

// writeJSON marshals v as 2-space-indented JSON and writes it atomically-ish
// (truncate-write; good enough for a developer tool). A trailing newline keeps
// POSIX tools and editors happy.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// sortedKeys returns the keys of m sorted, for deterministic iteration in the
// CLI output and lockfile assembly.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
