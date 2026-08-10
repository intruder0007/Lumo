package secretstore

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// osStat is os.Stat, indirected only so filestore_test.go's permission
// check reads through the same seam as production code.
var osStat = os.Stat

type fileStore struct {
	path string
}

// newFileStore returns a Store backed by a JSON file under the OS's
// standard per-user config directory, written with 0600 permissions.
// This is the never-fails-to-exist fallback when no native secret store
// is reachable (see New).
func newFileStore() (*fileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &fileStore{path: filepath.Join(dir, "lumo", "secrets.json")}, nil
}

func (f *fileStore) load() (map[string]string, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (f *fileStore) save(m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0o600)
}

func (f *fileStore) Set(key, secret string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	m[key] = secret
	return f.save(m)
}

func (f *fileStore) Get(key string) (string, bool, error) {
	m, err := f.load()
	if err != nil {
		return "", false, err
	}
	v, ok := m[key]
	return v, ok, nil
}

func (f *fileStore) Delete(key string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	delete(m, key)
	return f.save(m)
}
