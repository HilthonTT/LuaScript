package pkgmanager

import (
	"encoding/json"
	"os"
)

// Lock is the parsed `luascript-lock.json`. It records, per installed package
// name, exactly what was fetched so a later `install` reproduces the same
// bytes regardless of where the upstream ref now points.
type Lock struct {
	Packages map[string]LockEntry `json:"packages"`
}

// LockEntry is one resolved dependency. Source is the `host/path` (no ref),
// Ref is the tag/branch/commit the user asked for ("" = default branch), URL
// is the clone URL git was given, and Commit is the exact resolved hash.
type LockEntry struct {
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	URL    string `json:"url"`
	Commit string `json:"commit"`
}

// LoadLock reads the lockfile at path. A missing file yields an empty lock.
func LoadLock(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Lock{Packages: map[string]LockEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if l.Packages == nil {
		l.Packages = map[string]LockEntry{}
	}
	return &l, nil
}

// Save writes the lockfile as indented JSON.
func (l *Lock) Save(path string) error {
	return writeJSON(path, l)
}
