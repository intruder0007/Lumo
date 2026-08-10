// Package install implements the foundation layer for `lumo
// install`/`lumo uninstall`: copying the running binary to a per-user
// install location and keeping it on PATH. No admin/sudo privileges
// are required — everything lives under the current user's own
// profile, matching how `go install`, cargo, and pipx already place
// binaries. This is deliberately the CLI-subcommand half of the
// installer story (see docs/architecture/v1-readiness-report.md's
// v0.5.0 milestone); a future native installer.exe wraps this same
// logic rather than reimplementing it.
package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Options controls Install/Uninstall behavior.
type Options struct {
	// DryRun reports what would happen without changing anything on
	// disk or in the registry/shell config.
	DryRun bool
}

// Result summarizes what an operation did (or, in dry-run mode, would
// do) as a human-readable, ordered action list, plus whether anything
// actually changed — printing Actions is enough to show the user
// exactly what happened without the caller needing to know the
// implementation details.
type Result struct {
	BinaryPath string
	Actions    []string
	Changed    bool
}

// TargetDir returns the default per-user install directory for the
// current OS. Exported (not baked into Install) so callers — and
// tests — decide explicitly where on disk this touches; Install
// itself never resolves a default on its own.
func TargetDir() (string, error) {
	return targetDirOS()
}

// Install copies srcBinary (typically the result of os.Executable())
// into dir as the platform's lumo binary name, backing up an existing
// binary there first (name.bak) rather than silently overwriting — a
// safe-upgrade guarantee, not just a copy. A binary already identical
// to dst (same file, e.g. re-running install from an already-installed
// copy) is a no-op.
func Install(srcBinary, dir string, opts Options) (Result, error) {
	dst := filepath.Join(dir, binaryName())
	res := Result{BinaryPath: dst}

	same, err := sameFile(srcBinary, dst)
	if err != nil {
		return res, err
	}
	if same {
		res.Actions = append(res.Actions, fmt.Sprintf("already installed at %s (up to date)", dst))
		return res, nil
	}

	if _, err := os.Stat(dst); err == nil {
		bak := dst + ".bak"
		res.Actions = append(res.Actions, fmt.Sprintf("back up existing binary: %s -> %s", dst, bak))
		if !opts.DryRun {
			if err := os.Rename(dst, bak); err != nil {
				return res, fmt.Errorf("backing up existing binary: %w", err)
			}
		}
	}

	res.Actions = append(res.Actions, fmt.Sprintf("copy %s -> %s", srcBinary, dst))
	if !opts.DryRun {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return res, fmt.Errorf("creating install directory: %w", err)
		}
		if err := copyFile(srcBinary, dst); err != nil {
			return res, fmt.Errorf("copying binary: %w", err)
		}
	}
	res.Changed = true
	return res, nil
}

// Uninstall removes the binary at dir (and dir itself, if it's now
// empty). Uninstalling twice is not an error — nothing installed
// there is reported as a no-op action, not a failure.
func Uninstall(dir string, opts Options) (Result, error) {
	target := filepath.Join(dir, binaryName())
	res := Result{BinaryPath: target}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		res.Actions = append(res.Actions, fmt.Sprintf("nothing installed at %s", target))
		return res, nil
	} else if err != nil {
		return res, err
	}

	res.Actions = append(res.Actions, fmt.Sprintf("remove %s", target))
	if !opts.DryRun {
		if err := os.Remove(target); err != nil {
			return res, fmt.Errorf("removing binary: %w", err)
		}
	}
	res.Changed = true

	if empty, _ := dirEmpty(dir); empty {
		res.Actions = append(res.Actions, fmt.Sprintf("remove empty directory %s", dir))
		if !opts.DryRun {
			_ = os.Remove(dir) // best-effort: a non-empty race loser is fine to leave
		}
	}
	return res, nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "lumo.exe"
	}
	return "lumo"
}

// sameFile reports whether a and b are the same file on disk (not
// just equal paths) — os.SameFile handles symlinks/junctions
// correctly, which a plain string comparison wouldn't. b not existing
// is not an error: it just means they aren't the same file.
func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}

// copyFile writes to a temp file in dst's directory and renames over
// dst, so a failed/interrupted copy never leaves a half-written
// binary at the final path.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
