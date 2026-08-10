//go:build !windows

package secretstore

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// service namespaces every secret this package stores, so Lumo's entries
// in the OS secret store are identifiable and don't collide with other
// tools' entries.
const service = "lumo"

// errExitStatus44 is a sentinel used by tests to simulate "item not
// found" without needing the real security/secret-tool binaries
// installed. Production code never constructs this value directly — see
// isNotFound.
var errExitStatus44 = errors.New("exit status 44")

// cmdRunner abstracts exec.Command so macStore/linuxStore are testable
// without the real security/secret-tool binaries (see fakeRunner in
// unix_test.go).
type cmdRunner interface {
	Run(name string, args []string, stdin string) (stdout string, err error)
}

type execRunner struct{}

func (execRunner) Run(name string, args []string, stdin string) (string, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	return string(out), err
}

// nativeStore returns the current platform's native secret-store backend,
// if one is reachable. On Windows this file isn't compiled at all — see
// windows.go, which defines the same nativeStore symbol under a windows
// build tag.
func nativeStore() (Store, bool) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("security"); err != nil {
			return nil, false
		}
		return &macStore{run: execRunner{}}, true
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return nil, false
		}
		return &linuxStore{run: execRunner{}}, true
	default:
		return nil, false
	}
}

// isNotFound treats any command failure as "not found" rather than a hard
// error: both `security find-generic-password` (exit 44) and
// `secret-tool lookup` (nonzero, empty stdout) fail this way when the key
// simply isn't set, and cmdRunner's (stdout, err) signature doesn't carry
// enough detail to distinguish that from a genuinely broken binary. For a
// status-display feature, degrading a real infrastructure failure to "not
// configured" is an accepted, low-severity simplification — see plan
// Task 1 commentary.
func isNotFound(err error) bool { return err != nil }

type macStore struct{ run cmdRunner }

func (s *macStore) Set(key, secret string) error {
	_, err := s.run.Run("security", []string{"add-generic-password", "-a", service, "-s", key, "-w", secret, "-U"}, "")
	return err
}

func (s *macStore) Get(key string) (string, bool, error) {
	out, err := s.run.Run("security", []string{"find-generic-password", "-a", service, "-s", key, "-w"}, "")
	if isNotFound(err) {
		return "", false, nil
	}
	return strings.TrimRight(out, "\n"), true, nil
}

func (s *macStore) Delete(key string) error {
	_, err := s.run.Run("security", []string{"delete-generic-password", "-a", service, "-s", key}, "")
	return err
}

type linuxStore struct{ run cmdRunner }

func (s *linuxStore) Set(key, secret string) error {
	_, err := s.run.Run("secret-tool", []string{"store", "--label", "Lumo: " + key, "service", service, "key", key}, secret)
	return err
}

func (s *linuxStore) Get(key string) (string, bool, error) {
	out, err := s.run.Run("secret-tool", []string{"lookup", "service", service, "key", key}, "")
	if isNotFound(err) {
		return "", false, nil
	}
	return out, true, nil
}

func (s *linuxStore) Delete(key string) error {
	_, err := s.run.Run("secret-tool", []string{"clear", "service", service, "key", key}, "")
	return err
}
