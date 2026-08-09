//go:build windows

package secretstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsStore struct {
	path string
}

// nativeStore on Windows always succeeds: DPAPI (via CryptProtectData) is
// part of the OS, not an optional external tool like macOS's `security`
// or Linux's `secret-tool`.
func nativeStore() (Store, bool) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, false
	}
	return &windowsStore{path: filepath.Join(dir, "lumo", "secrets.dpapi.json")}, true
}

// protect encrypts plain with DPAPI, scoped to the current Windows user
// account — only that user (on this machine) can decrypt it.
func protect(plain []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(plain))}
	if len(plain) > 0 {
		in.Data = &plain[0]
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func unprotect(cipher []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(cipher))}
	if len(cipher) > 0 {
		in.Data = &cipher[0]
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func (w *windowsStore) load() (map[string][]byte, error) {
	data, err := os.ReadFile(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}
	m := map[string][]byte{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (w *windowsStore) save(m map[string][]byte) error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.path, data, 0o600)
}

func (w *windowsStore) Set(key, secret string) error {
	cipher, err := protect([]byte(secret))
	if err != nil {
		return err
	}
	m, err := w.load()
	if err != nil {
		return err
	}
	m[key] = cipher
	return w.save(m)
}

func (w *windowsStore) Get(key string) (string, bool, error) {
	m, err := w.load()
	if err != nil {
		return "", false, err
	}
	cipher, ok := m[key]
	if !ok {
		return "", false, nil
	}
	plain, err := unprotect(cipher)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}

func (w *windowsStore) Delete(key string) error {
	m, err := w.load()
	if err != nil {
		return err
	}
	delete(m, key)
	return w.save(m)
}
