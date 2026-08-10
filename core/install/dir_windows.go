//go:build windows

package install

import (
	"os"
	"path/filepath"
)

// targetDirOS returns %LocalAppData%\Programs\Lumo — per-user, no
// admin needed, and the same "Programs" convention VS Code/other
// user-scope Windows installers already use. os.UserCacheDir() on
// Windows returns %LocalAppData% (Go's stdlib documents this), which
// avoids hardcoding the env var name directly. Deliberately a sibling
// of, not the same directory as, os.UserCacheDir()/lumo/<version> —
// the embedded-plugin cache in cli/main.go's embeddedCacheDir(): that
// path is a disposable cache; this one holds an actually-installed
// binary.
func targetDirOS() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Programs", "Lumo"), nil
}
