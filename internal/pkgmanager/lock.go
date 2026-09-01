package pkgmanager

import (
	"encoding/json"
	"os"
)

type Lock struct {
	Packages map[string]LockEntry `json:"packages"`
}

type LockEntry struct {
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	URL    string `json:"url"`
	Commit string `json:"commit"`
}

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

func (l *Lock) Save(path string) error {
	return writeJSON(path, l)
}
