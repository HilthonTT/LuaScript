package pkgmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

const ManifestName = "luascript.json"

const LockName = "luascript-lock.json"

const ModulesDir = "lua_modules"

type Manifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

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

func (m *Manifest) Save(path string) error {
	return writeJSON(path, m)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
