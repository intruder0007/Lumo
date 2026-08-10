//go:build !windows

package install

import (
	"os"
	"path/filepath"
)

// targetDirOS returns ~/.local/bin — a common per-user bin directory
// on Linux/macOS (pipx, pip --user, and many distros' default PATH
// already include it), no sudo/admin needed.
func targetDirOS() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}
