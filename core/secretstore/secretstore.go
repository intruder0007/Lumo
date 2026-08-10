// Package secretstore persists small secrets (SonarQube API tokens, etc.)
// using the host OS's native secret store where one is reachable, falling
// back to a local file when it isn't. See
// docs/superpowers/specs/2026-08-09-security-connectors-phase-status-design.md
// Section 2.1 — callers must warn the user when New reports native=false,
// since the fallback is not encrypted at rest.
package secretstore

// Store persists and retrieves named secrets under a single, fixed
// namespace ("lumo") in whichever backend New selected.
type Store interface {
	// Set stores secret under key, overwriting any existing value.
	Set(key, secret string) error
	// Get retrieves the secret stored under key. found is false, err is
	// nil, if no secret has been stored under key — that is not an error.
	Get(key string) (secret string, found bool, err error)
	// Delete removes the secret stored under key. Deleting a key that
	// doesn't exist is not an error.
	Delete(key string) error
}

// New returns the best available Store for the current OS: a native OS
// secret store if one is reachable (nativeStore, implemented per-platform
// in unix.go/windows.go), otherwise a file-based fallback. native reports
// which one was chosen, so callers can warn the user when falling back to
// unencrypted-at-rest storage.
func New() (store Store, native bool, err error) {
	if s, ok := nativeStore(); ok {
		return s, true, nil
	}
	fs, err := newFileStore()
	if err != nil {
		return nil, false, err
	}
	return fs, false, nil
}
