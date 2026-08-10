# Security Features, GitHub/SonarQube Connectors, Phase Engine, and `lumo status` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add encrypted-at-rest secret storage, plugin-execution consent, an outbound-only GitHub/SonarQube connector layer built on a phase engine, and a new `lumo status` command that visually surfaces repo/Git/GitHub/SonarQube state — implementing Sections 2–4 of `docs/superpowers/specs/2026-08-09-security-connectors-phase-status-design.md`.

**Architecture:** A new `core/secretstore` package (OS-native secret store, file fallback) underneath a new `core/connector` package (phase engine + `GitHubConnector`/`SonarQubeConnector`, both outbound-only), wired into `cli` via a new `lumo status` command and extensions to the existing `lumo config`/`lumo new` commands. Module boundary is preserved: `cli → core → sdk/go`.

**Tech Stack:** Go 1.22, stdlib `net/http`/`os/exec`, `golang.org/x/sys/windows` (already a transitive dependency via `cli`, now also a direct dependency of `core` for DPAPI — no new third-party dependency family introduced).

## Global Constraints

- No new third-party Go dependencies beyond `golang.org/x/sys` (already present in the module graph via `cli`).
- Every outbound network call (GitHub via `gh`, SonarQube via HTTP) must print a `→ network: ...` line before firing, suppressible with `--quiet` (spec Section 2.3).
- `lumo status --offline` must skip all connector calls and show "offline — not checked" for GitHub/SonarQube rows instead of guessing (spec Section 2.3).
- Non-interactive/CI runs (existing ADR-0007 contract: `--answers`, piped stdin, `--yes`) must never block on a new prompt (spec Section 2.2).
- Secrets are never accepted as plaintext CLI flags (spec Section 2.1) — `lumo config set sonarqube-token` is interactive-only.
- Follow existing package doc-comment style (see `core/diag`, `core/registry`) — exported types/functions get a one-sentence doc comment explaining *why*, not just *what*.

---

### Task 1: `core/secretstore` — Store interface, file fallback, macOS/Linux native backends

**Files:**
- Create: `core/secretstore/secretstore.go`
- Create: `core/secretstore/filestore.go`
- Create: `core/secretstore/unix.go`
- Test: `core/secretstore/filestore_test.go`
- Test: `core/secretstore/unix_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `secretstore.Store` interface (`Set(key, secret string) error`, `Get(key string) (secret string, found bool, err error)`, `Delete(key string) error`) and `secretstore.New() (store Store, native bool, err error)` — consumed by Task 2 (adds the Windows backend to the same `New()` dispatcher) and Task 5 (`SonarQubeConnector` reads the token via `Store.Get`).

- [ ] **Step 1: Write the failing tests for the file-fallback store**

```go
// core/secretstore/filestore_test.go
package secretstore

import (
	"runtime"
	"testing"
)

func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	withTempConfigDir(t)
	fs, err := newFileStore()
	if err != nil {
		t.Fatalf("newFileStore: %v", err)
	}

	if _, found, err := fs.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get on empty store: found=%v err=%v, want found=false err=nil", found, err)
	}

	if err := fs.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, found, err := fs.Get("sonarqube-token")
	if err != nil || !found || got != "abc123" {
		t.Fatalf("Get after Set: got=%q found=%v err=%v, want got=%q found=true err=nil", got, found, err, "abc123")
	}

	if err := fs.Delete("sonarqube-token"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := fs.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get after Delete: found=%v err=%v, want found=false err=nil", found, err)
	}
}

func TestFileStorePermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
	withTempConfigDir(t)
	fs, err := newFileStore()
	if err != nil {
		t.Fatalf("newFileStore: %v", err)
	}
	if err := fs.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	info, err := osStat(fs.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secrets file mode = %o, want 0600", perm)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail (package doesn't exist yet)**

Run: `cd core && go test ./secretstore/... -run TestFileStore -v`
Expected: FAIL — `no such file or directory` / build error (package `secretstore` has no `newFileStore`, `osStat`).

- [ ] **Step 3: Implement the `Store` interface and file fallback**

```go
// core/secretstore/secretstore.go

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
```

```go
// core/secretstore/filestore.go
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
```

- [ ] **Step 4: Run the file-store tests to verify they pass**

Run: `cd core && go test ./secretstore/... -run TestFileStore -v`
Expected: PASS (both tests).

- [ ] **Step 5: Write the failing tests for the macOS/Linux native backends**

```go
// core/secretstore/unix_test.go
package secretstore

import (
	"slices"
	"testing"
)

type fakeRunner struct {
	calls  [][]string
	stdout string
	err    error
}

func (f *fakeRunner) Run(name string, args []string, stdin string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.stdout, f.err
}

func TestMacStoreSetBuildsExpectedCommand(t *testing.T) {
	r := &fakeRunner{}
	s := &macStore{run: r}
	if err := s.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"security", "add-generic-password", "-a", "lumo", "-s", "sonarqube-token", "-w", "abc123", "-U"}
	if len(r.calls) != 1 || !slices.Equal(r.calls[0], want) {
		t.Errorf("got calls %v, want one call %v", r.calls, want)
	}
}

func TestMacStoreGetNotFoundIsNotAnError(t *testing.T) {
	r := &fakeRunner{err: errExitStatus44}
	s := &macStore{run: r}
	secret, found, err := s.Get("sonarqube-token")
	if err != nil || found || secret != "" {
		t.Errorf("Get on missing key: secret=%q found=%v err=%v, want secret=\"\" found=false err=nil", secret, found, err)
	}
}

func TestLinuxStoreSetPassesSecretOnStdin(t *testing.T) {
	r := &fakeRunner{}
	s := &linuxStore{run: r}
	if err := s.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	wantArgs := []string{"secret-tool", "store", "--label", "Lumo: sonarqube-token", "service", "lumo", "key", "sonarqube-token"}
	if len(r.calls) != 1 || !slices.Equal(r.calls[0], wantArgs) {
		t.Errorf("got calls %v, want one call %v", r.calls, wantArgs)
	}
}

func TestLinuxStoreGetNotFoundIsNotAnError(t *testing.T) {
	r := &fakeRunner{err: errExitStatus44}
	s := &linuxStore{run: r}
	secret, found, err := s.Get("sonarqube-token")
	if err != nil || found || secret != "" {
		t.Errorf("Get on missing key: secret=%q found=%v err=%v, want secret=\"\" found=false err=nil", secret, found, err)
	}
}
```

- [ ] **Step 6: Run the tests to verify they fail**

Run: `cd core && go test ./secretstore/... -run 'TestMacStore|TestLinuxStore' -v`
Expected: FAIL — build error (`macStore`, `linuxStore`, `errExitStatus44` undefined).

- [ ] **Step 7: Implement the macOS/Linux native backends**

```go
// core/secretstore/unix.go
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
```

- [ ] **Step 8: Run the native-backend tests to verify they pass**

Run: `cd core && go test ./secretstore/... -v`
Expected: PASS (all `TestFileStore*`, `TestMacStore*`, `TestLinuxStore*` tests). This runs the full package test suite, since `unix.go`/`unix_test.go` have no build tag and compile on every OS (only the real `security`/`secret-tool` binaries are OS-specific, and tests never invoke them — they inject `fakeRunner`).

- [ ] **Step 9: Commit**

```bash
git add core/secretstore
git commit -m "feat: add secretstore package with file, macOS, and Linux backends"
```

---

### Task 2: `core/secretstore` — Windows DPAPI native backend

**Files:**
- Create: `core/secretstore/windows.go`
- Test: `core/secretstore/windows_test.go`
- Modify: `core/go.mod` (add direct dependency `golang.org/x/sys`)
- Modify: `core/go.sum`

**Interfaces:**
- Consumes: `Store` interface from Task 1.
- Produces: a second `nativeStore() (Store, bool)` definition (Windows-only, via `//go:build windows`), completing `New()`'s dispatch — no new symbols consumed by later tasks beyond what Task 1 already exposed.

**Note:** this repo's CI (`.github/workflows/ci.yml`) runs on `ubuntu-latest` only, so this backend's tests will not execute there — they run locally on a Windows machine (this development environment is `win32`, so Step 5 below runs for real here). Flagged, not silently skipped: consider adding a `windows-latest` CI job in a future pass if Windows coverage in CI matters; out of scope for this plan.

- [ ] **Step 1: Add `golang.org/x/sys` as a direct dependency of `core`**

Run: `cd core && go get golang.org/x/sys@v0.44.0`

Verify `core/go.mod` now has `golang.org/x/sys v0.44.0` under `require` (not `// indirect`).

- [ ] **Step 2: Write the failing test for the DPAPI round trip**

```go
// core/secretstore/windows_test.go
//go:build windows

package secretstore

import "testing"

func TestWindowsStoreRoundTrip(t *testing.T) {
	withTempConfigDir(t)
	s, ok := nativeStore()
	if !ok {
		t.Fatal("nativeStore() on windows should always report ok=true")
	}

	if _, found, err := s.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get on empty store: found=%v err=%v, want found=false err=nil", found, err)
	}

	if err := s.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, found, err := s.Get("sonarqube-token")
	if err != nil || !found || got != "abc123" {
		t.Fatalf("Get after Set: got=%q found=%v err=%v, want got=%q found=true err=nil", got, found, err, "abc123")
	}

	if err := s.Delete("sonarqube-token"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := s.Get("sonarqube-token"); err != nil || found {
		t.Fatalf("Get after Delete: found=%v err=%v, want found=false err=nil", found, err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd core && go test ./secretstore/... -run TestWindowsStore -v`
Expected: FAIL — build error, since `windows.go` doesn't exist yet and `unix.go` (from Task 1) still has no build tag, so its own `nativeStore()` is the only one active on this machine — but the test itself references nothing new yet, so confirm the failure is specifically about missing DPAPI behavior once Step 4 lands, not a false pass caused by `unix.go`'s Linux/macOS-shaped `nativeStore()` silently satisfying the symbol. Treat "builds and runs against the wrong (unix) backend" as equally a failure state to resolve in Step 4.

- [ ] **Step 4: Restrict `unix.go` to non-Windows and implement the DPAPI backend**

First, add a build tag to the top of `core/secretstore/unix.go` (from Task 1) so it no longer compiles on Windows:

```go
//go:build !windows

package secretstore
```

(This is the one deferred piece from Task 1: `unix.go` was left untagged there because Task 1 had no Windows backend yet to conflict with. Adding the tag now is what makes both files coexist.)

Then create the DPAPI backend:

```go
// core/secretstore/windows.go
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
```

`encoding/json` marshals `[]byte` fields as base64 automatically, so no manual encoding step is needed for the ciphertext.

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd core && go test ./secretstore/... -v`
Expected: PASS for all tests, including `TestWindowsStoreRoundTrip` (this environment is Windows, so the `windows` build tag is active).

- [ ] **Step 6: Verify the non-Windows build still compiles (cross-compile check)**

Run: `cd core && GOOS=linux GOARCH=amd64 go build ./...`
Expected: succeeds — `windows.go` is excluded, `unix.go` (now tagged `!windows`) is included.

- [ ] **Step 7: Commit**

```bash
git add core/secretstore/windows.go core/secretstore/windows_test.go core/secretstore/unix.go core/go.mod core/go.sum
git commit -m "feat: add Windows DPAPI backend to secretstore"
```

---

### Task 3: `core/connector` — phase engine

**Files:**
- Create: `core/connector/connector.go`
- Test: `core/connector/connector_test.go`

**Interfaces:**
- Consumes: `core/diag.Logger`, `core/diag.NoopLogger` (existing, from the task-independent `core/diag` package).
- Produces: `connector.Phase{Name string; Run func() (Result, error)}`, `connector.Result{Connected bool; Detail string}`, `connector.Connector` interface (`Name() string`, `Phases() []Phase`), `connector.RunEngine(c Connector, logger diag.Logger) (Result, error)` — consumed by Task 4 (`GitHubConnector`), Task 5 (`SonarQubeConnector`), and Task 9 (`cmdStatus`).

- [ ] **Step 1: Write the failing tests for the phase engine**

```go
// core/connector/connector_test.go
package connector

import (
	"errors"
	"strings"
	"testing"

	"github.com/intruder0007/Lumo/core/diag"
)

type fakeConnector struct {
	name   string
	phases []Phase
}

func (f fakeConnector) Name() string    { return f.name }
func (f fakeConnector) Phases() []Phase { return f.phases }

func TestRunEngineRunsPhasesInOrderAndReturnsLastResult(t *testing.T) {
	var order []string
	c := fakeConnector{name: "test", phases: []Phase{
		{Name: "auth", Run: func() (Result, error) { order = append(order, "auth"); return Result{Connected: true}, nil }},
		{Name: "fetch", Run: func() (Result, error) { order = append(order, "fetch"); return Result{Connected: true}, nil }},
		{Name: "validate", Run: func() (Result, error) { order = append(order, "validate"); return Result{Connected: true, Detail: "ok"}, nil }},
	}}

	got, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if !got.Connected || got.Detail != "ok" {
		t.Errorf("RunEngine result = %+v, want Connected=true Detail=%q", got, "ok")
	}
	wantOrder := []string{"auth", "fetch", "validate"}
	if len(order) != len(wantOrder) {
		t.Fatalf("phase order = %v, want %v", order, wantOrder)
	}
	for i, name := range wantOrder {
		if order[i] != name {
			t.Errorf("phase order = %v, want %v", order, wantOrder)
			break
		}
	}
}

func TestRunEngineShortCircuitsOnFailureNamingThePhase(t *testing.T) {
	var ran []string
	c := fakeConnector{name: "sonarqube", phases: []Phase{
		{Name: "auth", Run: func() (Result, error) {
			ran = append(ran, "auth")
			return Result{}, errors.New("token rejected (401)")
		}},
		{Name: "fetch", Run: func() (Result, error) {
			ran = append(ran, "fetch")
			return Result{Connected: true}, nil
		}},
	}}

	_, err := RunEngine(c, diag.NoopLogger{})
	if err == nil {
		t.Fatal("RunEngine: want error, got nil")
	}
	if !strings.Contains(err.Error(), "sonarqube") || !strings.Contains(err.Error(), "auth") || !strings.Contains(err.Error(), "token rejected (401)") {
		t.Errorf("RunEngine error = %q, want it to name the connector, phase, and underlying error", err.Error())
	}
	if len(ran) != 1 || ran[0] != "auth" {
		t.Errorf("phases run = %v, want only [auth] (short-circuit)", ran)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core && go test ./connector/... -v`
Expected: FAIL — build error (package `connector` doesn't exist yet).

- [ ] **Step 3: Implement the phase engine**

```go
// core/connector/connector.go

// Package connector runs outbound-only integrations (GitHub, SonarQube)
// as an ordered sequence of named phases (auth, fetch, validate, ...),
// short-circuiting on the first failure with a message naming which phase
// failed. See
// docs/superpowers/specs/2026-08-09-security-connectors-phase-status-design.md
// Section 3. This phase shape is scoped to connector operations only — it
// is not a general build/rollout-phase system for the rest of Lumo.
package connector

import (
	"fmt"

	"github.com/intruder0007/Lumo/core/diag"
)

// Result is what a connector reports after its phases finish (or after
// the phase engine short-circuits — see RunEngine).
type Result struct {
	Connected bool
	Detail    string
}

// Phase is one named step of a connector's operation (e.g. "auth",
// "fetch", "validate", "display"). Run performs the step; its returned
// Result is only meaningful for the last phase to run — RunEngine returns
// whatever the final executed phase returned.
type Phase struct {
	Name string
	Run  func() (Result, error)
}

// Connector is an outbound integration (GitHub, SonarQube) expressed as
// an ordered list of phases.
type Connector interface {
	Name() string
	Phases() []Phase
}

// PhaseError names which connector and phase failed, wrapping the
// underlying error so callers (e.g. `lumo status`) can print a specific
// message like "sonarqube: failed at auth: token rejected (401)" instead
// of a bare error.
type PhaseError struct {
	Connector, Phase string
	Err              error
}

func (e *PhaseError) Error() string {
	return fmt.Sprintf("%s: failed at %s: %v", e.Connector, e.Phase, e.Err)
}
func (e *PhaseError) Unwrap() error { return e.Err }

// RunEngine runs c's phases strictly in order, logging each phase
// transition through logger (see core/diag), and stops at the first
// phase that returns an error.
func RunEngine(c Connector, logger diag.Logger) (Result, error) {
	var last Result
	for _, p := range c.Phases() {
		logger.Logf("%s: phase %s starting", c.Name(), p.Name)
		res, err := p.Run()
		if err != nil {
			logger.Logf("%s: phase %s failed: %v", c.Name(), p.Name, err)
			return Result{}, &PhaseError{Connector: c.Name(), Phase: p.Name, Err: err}
		}
		logger.Logf("%s: phase %s done", c.Name(), p.Name)
		last = res
	}
	return last, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core && go test ./connector/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/connector/connector.go core/connector/connector_test.go
git commit -m "feat: add connector package with ordered phase engine"
```

---

### Task 4: `core/connector` — `GitHubConnector`

**Files:**
- Create: `core/connector/github.go`
- Test: `core/connector/github_test.go`

**Interfaces:**
- Consumes: `Phase`, `Result`, `Connector` from Task 3.
- Produces: `connector.NewGitHubConnector(run cmdRunner) *GitHubConnector` (unexported `cmdRunner` type, same shape as `secretstore`'s but independently defined here — `core/connector` doesn't import `core/secretstore`, keeping the two packages decoupled) — consumed by Task 9 (`cmdStatus`, using the real `execRunner`).

- [ ] **Step 1: Write the failing tests**

```go
// core/connector/github_test.go
package connector

import (
	"errors"
	"testing"

	"github.com/intruder0007/Lumo/core/diag"
)

type fakeGHRunner struct {
	responses map[string]struct {
		stdout string
		err    error
	}
}

func (f fakeGHRunner) Run(name string, args []string) (string, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	r, ok := f.responses[key]
	if !ok {
		return "", errors.New("unexpected command: " + key)
	}
	return r.stdout, r.err
}

func TestGitHubConnectorReportsConnectedWhenAuthenticated(t *testing.T) {
	run := fakeGHRunner{responses: map[string]struct {
		stdout string
		err    error
	}{
		"gh auth status": {stdout: "Logged in to github.com as octocat", err: nil},
	}}
	c := NewGitHubConnector(run)
	res, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if !res.Connected {
		t.Errorf("Connected = false, want true")
	}
	if res.Detail == "" {
		t.Errorf("Detail is empty, want the account name surfaced")
	}
}

func TestGitHubConnectorReportsNotConnectedWhenGhMissing(t *testing.T) {
	run := fakeGHRunner{responses: map[string]struct {
		stdout string
		err    error
	}{}}
	c := NewGitHubConnector(run)
	res, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if res.Connected {
		t.Errorf("Connected = true, want false when gh is unavailable")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core && go test ./connector/... -run TestGitHubConnector -v`
Expected: FAIL — build error (`NewGitHubConnector`, `cmdRunner` undefined).

- [ ] **Step 3: Implement `GitHubConnector`**

```go
// core/connector/github.go
package connector

import (
	"os/exec"
	"strings"
)

// cmdRunner abstracts exec.Command so GitHubConnector is testable without
// the real gh binary (see fakeGHRunner in github_test.go). The real
// implementation is ExecCmdRunner, constructed by callers (Task 9's
// cmdStatus) rather than by this package, so core/connector's exported
// surface stays free of os/exec side effects at import time.
type cmdRunner interface {
	Run(name string, args []string) (stdout string, err error)
}

// ExecCmdRunner is the production cmdRunner, shelling out via os/exec.
type ExecCmdRunner struct{}

func (ExecCmdRunner) Run(name string, args []string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// GitHubConnector reports whether the `gh` CLI is authenticated. No
// GitHub token is ever handled by Lumo directly — this reuses whatever
// session `gh auth login` already established. If `gh` isn't installed or
// isn't authenticated, the connector reports "not connected", never an
// error (see the auth phase below).
type GitHubConnector struct {
	run cmdRunner
}

func NewGitHubConnector(run cmdRunner) *GitHubConnector {
	return &GitHubConnector{run: run}
}

func (c *GitHubConnector) Name() string { return "github" }

func (c *GitHubConnector) Phases() []Phase {
	return []Phase{
		{Name: "auth", Run: c.auth},
	}
}

func (c *GitHubConnector) auth() (Result, error) {
	out, err := c.run.Run("gh", []string{"auth", "status"})
	if err != nil {
		return Result{Connected: false, Detail: "gh not authenticated (or not installed)"}, nil
	}
	return Result{Connected: true, Detail: strings.TrimSpace(out)}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core && go test ./connector/... -run TestGitHubConnector -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/connector/github.go core/connector/github_test.go
git commit -m "feat: add GitHubConnector (gh CLI auth status)"
```

---

### Task 5: `core/connector` — `SonarQubeConnector`

**Files:**
- Create: `core/connector/sonarqube.go`
- Test: `core/connector/sonarqube_test.go`

**Interfaces:**
- Consumes: `Phase`, `Result`, `Connector` from Task 3.
- Produces: `connector.NewSonarQubeConnector(baseURL, token string, client *http.Client) *SonarQubeConnector` — consumed by Task 9 (`cmdStatus`), which supplies `baseURL` from `prompt.Config.SonarQubeURL` (Task 6) and `token` from `secretstore.Store.Get` (Task 1/2).

- [ ] **Step 1: Write the failing tests**

```go
// core/connector/sonarqube_test.go
package connector

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/intruder0007/Lumo/core/diag"
)

func TestSonarQubeConnectorReportsConnectedOnValidStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/system/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Errorf("Authorization header not set")
		}
		w.Write([]byte(`{"status":"UP"}`))
	}))
	defer srv.Close()

	c := NewSonarQubeConnector(srv.URL, "test-token", srv.Client())
	res, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if !res.Connected {
		t.Errorf("Connected = false, want true")
	}
}

func TestSonarQubeConnectorFailsAtAuthOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewSonarQubeConnector(srv.URL, "bad-token", srv.Client())
	_, err := RunEngine(c, diag.NoopLogger{})
	if err == nil {
		t.Fatal("RunEngine: want error on 401, got nil")
	}
	perr, ok := err.(*PhaseError)
	if !ok {
		t.Fatalf("error type = %T, want *PhaseError", err)
	}
	if perr.Phase != "auth" {
		t.Errorf("PhaseError.Phase = %q, want %q", perr.Phase, "auth")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd core && go test ./connector/... -run TestSonarQubeConnector -v`
Expected: FAIL — build error (`NewSonarQubeConnector` undefined).

- [ ] **Step 3: Implement `SonarQubeConnector`**

```go
// core/connector/sonarqube.go
package connector

import (
	"fmt"
	"net/http"
)

// SonarQubeConnector checks reachability and auth against a configured
// SonarQube server (self-hosted or SonarCloud — both expose the same
// /api/system/status shape). The token comes from core/secretstore via
// the caller (Task 9's cmdStatus); this package never touches secret
// storage directly, keeping connector and secretstore independently
// testable.
type SonarQubeConnector struct {
	baseURL, token string
	client         *http.Client
}

func NewSonarQubeConnector(baseURL, token string, client *http.Client) *SonarQubeConnector {
	if client == nil {
		client = http.DefaultClient
	}
	return &SonarQubeConnector{baseURL: baseURL, token: token, client: client}
}

func (c *SonarQubeConnector) Name() string { return "sonarqube" }

func (c *SonarQubeConnector) Phases() []Phase {
	return []Phase{
		{Name: "auth", Run: c.auth},
	}
}

func (c *SonarQubeConnector) auth() (Result, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/system/status", nil)
	if err != nil {
		return Result{}, err
	}
	req.SetBasicAuth(c.token, "")
	resp, err := c.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return Result{}, fmt.Errorf("token rejected (401)")
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return Result{Connected: true, Detail: c.baseURL}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd core && go test ./connector/... -v`
Expected: PASS (all of Tasks 3–5's tests).

- [ ] **Step 5: Commit**

```bash
git add core/connector/sonarqube.go core/connector/sonarqube_test.go
git commit -m "feat: add SonarQubeConnector (system status auth check)"
```

---

### Task 6: extend `prompt.Config` with SonarQube URL and plugin approvals

**Files:**
- Modify: `cli/internal/prompt/config.go`
- Modify: `cli/internal/prompt/config_test.go`

**Interfaces:**
- Consumes: existing `prompt.Config`, `LoadConfig`, `SaveConfig` (task-independent, already in the codebase).
- Produces: `Config.SonarQubeURL string` and `Config.ApprovedPlugins []string` fields — consumed by Task 7 (`lumo config set sonarqube-url`) and Task 8 (plugin consent, appends to `ApprovedPlugins`).

- [ ] **Step 1: Write the failing test extension**

Add to `cli/internal/prompt/config_test.go` (new test function, existing `TestConfigRoundTrip` untouched):

```go
func TestConfigRoundTripSonarQubeAndApprovedPlugins(t *testing.T) {
	withTempConfigDir(t)

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig on a missing file should not error, got: %v", err)
	}
	if got.SonarQubeURL != "" {
		t.Errorf("LoadConfig with no saved file: got SonarQubeURL=%q, want empty", got.SonarQubeURL)
	}
	if len(got.ApprovedPlugins) != 0 {
		t.Errorf("LoadConfig with no saved file: got ApprovedPlugins=%v, want empty", got.ApprovedPlugins)
	}

	cfg := Config{
		SonarQubeURL:    "https://sonar.example.com",
		ApprovedPlugins: []string{"git-init@1.0.0@/path/to/git-init"},
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if got.SonarQubeURL != cfg.SonarQubeURL {
		t.Errorf("LoadConfig after SaveConfig: got SonarQubeURL=%q, want %q", got.SonarQubeURL, cfg.SonarQubeURL)
	}
	if len(got.ApprovedPlugins) != 1 || got.ApprovedPlugins[0] != cfg.ApprovedPlugins[0] {
		t.Errorf("LoadConfig after SaveConfig: got ApprovedPlugins=%v, want %v", got.ApprovedPlugins, cfg.ApprovedPlugins)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd cli && go test ./internal/prompt/... -run TestConfigRoundTripSonarQubeAndApprovedPlugins -v`
Expected: FAIL — build error (`Config.SonarQubeURL`, `Config.ApprovedPlugins` undefined).

- [ ] **Step 3: Add the fields**

In `cli/internal/prompt/config.go`, modify the `Config` struct:

```go
type Config struct {
	Theme string `json:"theme,omitempty"`
	// DefaultProjectsDir is the last directory the interactive wizard's
	// location step was pointed at — offered as that step's editable
	// pre-fill on the next run (never silently applied without asking;
	// see wizard.go's stepLocation).
	DefaultProjectsDir string `json:"defaultProjectsDir,omitempty"`
	// SonarQubeURL is the configured SonarQube (self-hosted or
	// SonarCloud) server to check in `lumo status`. The auth token is
	// never stored here — see core/secretstore, keyed by "sonarqube-token".
	SonarQubeURL string `json:"sonarQubeURL,omitempty"`
	// ApprovedPlugins records plugins the user has already consented to
	// run, as "name@version@entrypointPath" entries, so `lumo new`/`lumo
	// plugins validate` only prompts once per plugin version+location
	// (see confirmPluginTrust in main.go).
	ApprovedPlugins []string `json:"approvedPlugins,omitempty"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd cli && go test ./internal/prompt/... -v`
Expected: PASS (both `TestConfigRoundTrip` and the new test).

- [ ] **Step 5: Commit**

```bash
git add cli/internal/prompt/config.go cli/internal/prompt/config_test.go
git commit -m "feat: add SonarQubeURL and ApprovedPlugins to persisted config"
```

---

### Task 7: `lumo config` — SonarQube URL and token subcommands

**Files:**
- Modify: `cli/main.go` (extend `configUsage`, `cmdConfig`; add `cmdConfigGetSonarQubeURL`, `cmdConfigSetSonarQubeURL`, `cmdConfigSetSonarQubeToken`)
- Modify: `cli/internal/prompt/screens.go:255-258` (extend `HelpText`'s `config` entries)

**Interfaces:**
- Consumes: `prompt.Config.SonarQubeURL` (Task 6), `secretstore.New`/`secretstore.Store` (Task 1/2).
- Produces: `lumo config get/set sonarqube-url`, `lumo config set sonarqube-token` — consumed by Task 9 (`cmdStatus` reads both to build the `SonarQubeConnector`).

- [ ] **Step 1: Add the new subcommands to `cmdConfig`'s dispatch**

In `cli/main.go`, modify `configUsage`:

```go
func configUsage() {
	fmt.Fprintln(os.Stderr, `usage: lumo config get theme
       lumo config set theme <default|minimal>
       lumo config get projects-dir
       lumo config set projects-dir <path>
       lumo config get sonarqube-url
       lumo config set sonarqube-url <url>
       lumo config set sonarqube-token   (interactive prompt; never accepts the token as an argument)`)
}
```

Modify `cmdConfig`'s switch to add the new `key` cases:

```go
func cmdConfig(args []string) {
	if len(args) < 2 || (args[0] != "get" && args[0] != "set") {
		configUsage()
		exit(1)
	}
	key, action := args[1], args[0]
	switch key {
	case "theme":
		if action == "get" {
			cmdConfigGetTheme()
		} else {
			cmdConfigSetTheme(args[2:])
		}
	case "projects-dir":
		if action == "get" {
			cmdConfigGetProjectsDir()
		} else {
			cmdConfigSetProjectsDir(args[2:])
		}
	case "sonarqube-url":
		if action == "get" {
			cmdConfigGetSonarQubeURL()
		} else {
			cmdConfigSetSonarQubeURL(args[2:])
		}
	case "sonarqube-token":
		if action == "get" {
			fmt.Fprintln(os.Stderr, "error: sonarqube-token cannot be read back (write-only; use 'lumo status' to check it's configured)")
			exit(2)
		}
		cmdConfigSetSonarQubeToken()
	default:
		configUsage()
		exit(1)
	}
}
```

Add the three new functions after `cmdConfigSetProjectsDir`:

```go
func cmdConfigGetSonarQubeURL() {
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	fmt.Println(cfg.SonarQubeURL)
}

func cmdConfigSetSonarQubeURL(rest []string) {
	if len(rest) < 1 || rest[0] == "" {
		configUsage()
		exit(1)
	}
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	cfg.SonarQubeURL = rest[0]
	if err := prompt.SaveConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
}

// cmdConfigSetSonarQubeToken reads the token interactively (never as a
// CLI argument, to avoid shell-history/process-list leakage — spec
// Section 2.1) and stores it via secretstore, warning if the OS has no
// native secret store reachable and Lumo is falling back to an
// unencrypted local file.
func cmdConfigSetSonarQubeToken() {
	fmt.Print("SonarQube token: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	token := strings.TrimSpace(line)
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: token cannot be empty")
		exit(1)
	}

	store, native, err := secretstore.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	if !native {
		fmt.Fprintln(os.Stderr, "warning: no OS secret store available — the token will be stored in a local file, unencrypted at rest")
	}
	if err := store.Set("sonarqube-token", token); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	fmt.Println("SonarQube token saved.")
}
```

Add the new imports to `cli/main.go`'s import block: `"bufio"`, `"io"`, and `"github.com/intruder0007/Lumo/core/secretstore"`.

- [ ] **Step 2: Extend `HelpText`**

In `cli/internal/prompt/screens.go`, modify the `config` lines (255-258) to:

```
  config get theme      print the persisted theme (empty if unset)
  config set theme <name>
                        persist a theme (default|minimal) for future
                        interactive runs
  config get/set sonarqube-url <url>
                        configure the SonarQube server 'lumo status'
                        checks (self-hosted or SonarCloud)
  config set sonarqube-token
                        interactively set the SonarQube token (never
                        accepted as an argument); stored via the OS
                        secret store when available
```

- [ ] **Step 3: Manually verify the new subcommands**

Run: `cd cli && go build -o lumo-test.exe .`

Then:
```
./lumo-test.exe config set sonarqube-url https://sonar.example.com
./lumo-test.exe config get sonarqube-url
```
Expected: second command prints `https://sonar.example.com`.

```
echo test-token-123| ./lumo-test.exe config set sonarqube-token
```
Expected: prints `SonarQube token saved.` (plus the native-store warning line if this shell has no Windows Credential Manager access — acceptable either way, both paths are exercised by Task 1/2's own automated tests).

Delete `lumo-test.exe` afterward — not a repo artifact, matches how other ad hoc local builds in this workflow are handled.

- [ ] **Step 4: Run the full `cli` test suite to confirm nothing broke**

Run: `cd cli && go build ./... && go vet ./... && go test ./...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add cli/main.go cli/internal/prompt/screens.go
git commit -m "feat: add lumo config sonarqube-url/sonarqube-token subcommands"
```

---

### Task 8: plugin execution consent

**Files:**
- Modify: `cli/main.go` (add `--yes` flag to `cmdNew`, add `pluginTrustKey`/`isApproved`/`confirmPluginTrust`, call it before `eng.Run`)
- Test: `cli/main_test.go` (new test for the approval-key format helper)

**Interfaces:**
- Consumes: `registry.Registry.ResolveTemplate`/`ResolveCapability` (existing), `prompt.Config.ApprovedPlugins` (Task 6).
- Produces: `pluginTrustKey(p registry.Plugin) string`, `confirmPluginTrust(reg *registry.Registry, a config.Answers, yes bool) error` — used only within `cmdNew` in this plan; no later task depends on it.

- [ ] **Step 1: Write the failing test for the approval-key format**

Add to `cli/main_test.go`. First check its current imports (`cli/main_test.go` today imports only stdlib packages per its existing tests — `registry` and `sdk` are not yet imported there even though `main.go` uses both) and add:

```go
"github.com/intruder0007/Lumo/core/registry"
sdk "github.com/intruder0007/Lumo/sdk/go/sdk"
```

Then add the test:

```go
func TestPluginTrustKeyIncludesNameVersionAndPath(t *testing.T) {
	p := registry.Plugin{
		Manifest:       sdk.Manifest{Name: "git-init", Version: "1.0.0"},
		EntrypointPath: "/plugins/git-init/git-init",
	}
	got := pluginTrustKey(p)
	want := "git-init@1.0.0@/plugins/git-init/git-init"
	if got != want {
		t.Errorf("pluginTrustKey = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd cli && go test ./... -run TestPluginTrustKey -v`
Expected: FAIL — build error (`pluginTrustKey` undefined).

- [ ] **Step 3: Implement the consent check**

Add to `cli/main.go`:

```go
// pluginTrustKey identifies a specific plugin build for consent tracking:
// name+version+resolved path, so a plugin binary swapped at the same
// path (a stale/different build) or a version bump re-triggers consent —
// see SECURITY.md's "installing a plugin is consent to run it" model and
// spec Section 2.2.
func pluginTrustKey(p registry.Plugin) string {
	return p.Manifest.Name + "@" + p.Manifest.Version + "@" + p.EntrypointPath
}

func isApproved(cfg prompt.Config, key string) bool {
	for _, k := range cfg.ApprovedPlugins {
		if k == key {
			return true
		}
	}
	return false
}

// confirmPluginTrust resolves every plugin a.ProjectType/Language/
// Framework/Capabilities will run and, for any not already approved,
// prompts for confirmation (skipped entirely when yes is true — the
// existing non-interactive contract for --answers/CI runs, ADR-0007).
// Approvals are persisted to prompt.Config.ApprovedPlugins so the same
// plugin version+path never re-prompts.
func confirmPluginTrust(reg *registry.Registry, a config.Answers, yes bool) error {
	if yes {
		return nil
	}

	var toConfirm []registry.Plugin
	tmpl, err := reg.ResolveTemplate(a.ProjectType, a.Language, a.Framework)
	if err != nil {
		return err
	}
	toConfirm = append(toConfirm, tmpl)
	for _, capID := range a.Capabilities {
		capPlugin, err := reg.ResolveCapability(capID)
		if err != nil {
			return err
		}
		toConfirm = append(toConfirm, capPlugin)
	}

	cfg, err := prompt.LoadConfig()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	changed := false
	for _, p := range toConfirm {
		key := pluginTrustKey(p)
		if isApproved(cfg, key) {
			continue
		}
		fmt.Printf("About to run plugin %q v%s (%s). Continue? [y/N] ", p.Manifest.Name, p.Manifest.Version, p.EntrypointPath)
		line, _ := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("declined to run plugin %q", p.Manifest.Name)
		}
		cfg.ApprovedPlugins = append(cfg.ApprovedPlugins, key)
		changed = true
	}
	if changed {
		if err := prompt.SaveConfig(cfg); err != nil {
			return err
		}
	}
	return nil
}
```

In `cmdNew`, add a `-yes` flag next to the existing `-verbose`/`-v` declarations (around line 309-310):

```go
	yes := fs.Bool("yes", false, "skip plugin-execution confirmation prompts (implied by --answers and non-interactive runs)")
```

And also add `"-yes": true, "--yes": true` to the `extractProjectName` boolean-flags map at the top of `cmdNew` (alongside the existing `-no-color`/`-verbose` entries).

Then, immediately after `reg := registry.New(pluginDirs()...)` and before `host := plugin.NewHost()` (around line 458), insert:

```go
	nonInteractive := *yes || *answersFile != "" || !interactive
	if err := confirmPluginTrust(reg, a, nonInteractive); err != nil {
		prompt.ErrorScreen(os.Stdout, t, err)
		exit(1)
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd cli && go test ./... -v`
Expected: PASS (all existing tests plus the new one).

- [ ] **Step 5: Manually verify the prompt appears and is remembered**

```
cd cli && go build -o lumo-test.exe .
./lumo-test.exe new demo-app -project-type backend-service -language go -framework rest-api -dir C:\Temp
```
Expected: a confirmation prompt for the `go-rest-api` template plugin appears before generation starts; answering `y` proceeds; re-running the same command (against a fresh empty target dir) does not re-prompt.

Delete `lumo-test.exe` and the generated `C:\Temp\demo-app` directory afterward.

- [ ] **Step 6: Commit**

```bash
git add cli/main.go cli/main_test.go
git commit -m "feat: prompt for plugin-execution consent on first run of a plugin version"
```

---

### Task 9: `lumo status` — repo/Git/GitHub/SonarQube rows

**Files:**
- Modify: `cli/main.go` (add `case "status"`, `cmdStatus`, `statusUsage`, `printConnectorResult`)
- Modify: `cli/internal/prompt/screens.go` (extend `HelpText`)
- Test: `tests/integration/integration_test.go` (new end-to-end test, following the existing `TestEndToEndGenerate*` build-then-exec pattern)

**Interfaces:**
- Consumes: `connector.NewGitHubConnector`, `connector.NewSonarQubeConnector`, `connector.RunEngine`, `connector.ExecCmdRunner` (Task 3–5); `secretstore.New` (Task 1–2); `prompt.Config.SonarQubeURL` (Task 6).
- Produces: `lumo status [--offline] [--verbose]` — terminal command for this task; Task 10 extends it with one more row.

- [ ] **Step 1: Add the command dispatch and `HelpText` entry**

In `cli/main.go`'s `main()` switch, add:

```go
	case "status":
		cmdStatus(os.Args[2:])
```

In `cli/internal/prompt/screens.go`, add before the `version` line in `HelpText`:

```
  status                 show current repo/project, Git, GitHub, and
                        SonarQube connection state (run 'lumo status -h'
                        for flags)
```

- [ ] **Step 2: Implement `cmdStatus`**

Add to `cli/main.go`:

```go
func statusUsage() {
	fmt.Fprintln(os.Stderr, `usage: lumo status [flags]

Shows the current repo/project, whether Git is initialized, and whether
GitHub and SonarQube are reachable/configured.`)
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = statusUsage
	offline := fs.Bool("offline", false, "skip GitHub/SonarQube network checks")
	verbose := fs.Bool("verbose", false, "print phase-by-phase connector logging to stderr")
	fs.BoolVar(verbose, "v", false, "shorthand for -verbose")
	fs.Parse(args)

	cfg, _ := prompt.LoadConfig()
	themeName := prompt.ResolveThemeName("", cfg.Theme)
	t := prompt.GetTheme(themeName, os.Getenv("NO_COLOR") != "")

	var logger diag.Logger = diag.NoopLogger{}
	if *verbose {
		logger = diag.WriterLogger{W: os.Stderr}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	projectName := filepath.Base(cwd)
	fmt.Println(t.Header("Project:"))
	fmt.Println(t.Success(fmt.Sprintf("%s (%s)", projectName, cwd)))
	fmt.Println()

	fmt.Println(t.Header("Git:"))
	if info, statErr := os.Stat(filepath.Join(cwd, ".git")); statErr == nil && info.IsDir() {
		fmt.Println(t.Success("initialized"))
	} else {
		fmt.Println(t.Failure("not initialized"))
	}
	fmt.Println()

	fmt.Println(t.Header("GitHub:"))
	if *offline {
		fmt.Println(t.Dim("offline — not checked"))
	} else {
		fmt.Fprintln(os.Stderr, "→ network: GitHub API (gh auth status)")
		gh := connector.NewGitHubConnector(connector.ExecCmdRunner{})
		res, err := connector.RunEngine(gh, logger)
		printConnectorResult(t, res, err)
	}
	fmt.Println()

	fmt.Println(t.Header("SonarQube:"))
	switch {
	case *offline:
		fmt.Println(t.Dim("offline — not checked"))
	case cfg.SonarQubeURL == "":
		fmt.Println(t.Dim("not configured (see 'lumo config set sonarqube-url')"))
	default:
		store, native, storeErr := secretstore.New()
		if storeErr != nil {
			fmt.Println(t.Failure("error reading token: " + storeErr.Error()))
			break
		}
		if !native {
			fmt.Fprintln(os.Stderr, "warning: SonarQube token is stored in an unencrypted local file (no OS secret store available)")
		}
		token, found, getErr := store.Get("sonarqube-token")
		if getErr != nil {
			fmt.Println(t.Failure("error reading token: " + getErr.Error()))
			break
		}
		if !found {
			fmt.Println(t.Dim("not configured (see 'lumo config set sonarqube-token')"))
			break
		}
		fmt.Fprintln(os.Stderr, "→ network: SonarQube ("+cfg.SonarQubeURL+"/api/system/status)")
		sq := connector.NewSonarQubeConnector(cfg.SonarQubeURL, token, nil)
		res, err := connector.RunEngine(sq, logger)
		printConnectorResult(t, res, err)
	}
}

func printConnectorResult(t prompt.Theme, res connector.Result, err error) {
	if err != nil {
		fmt.Println(t.Failure(err.Error()))
		return
	}
	if res.Connected {
		fmt.Println(t.Success(res.Detail))
	} else {
		fmt.Println(t.Failure(res.Detail))
	}
}
```

Add `"github.com/intruder0007/Lumo/core/connector"` to `cli/main.go`'s import block (`secretstore` was already added in Task 7).

- [ ] **Step 3: Write the failing end-to-end test**

`tests/integration/integration_test.go` already has the exact helpers this test needs, used by `TestEndToEndGenerateGoRestAPI` (lines 62-67): `repoRoot(t) string`, `exeName(base string) string`, and `buildBinary(t *testing.T, root, pkgDir, outPath string)`. `lumo status` doesn't touch plugin discovery, so this test only needs to build the `cli` binary itself — no template/capability plugin setup.

Add to `tests/integration/integration_test.go`:

```go
func TestStatusOfflineShowsRepoAndGitOnly(t *testing.T) {
	root := repoRoot(t)
	bin := t.TempDir()
	cliPath := filepath.Join(bin, exeName("lumo"))
	buildBinary(t, root, "cli", cliPath)

	dir := t.TempDir()
	cmd := exec.Command(cliPath, "status", "--offline")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lumo status --offline failed: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "Project:") {
		t.Errorf("output missing Project section:\n%s", got)
	}
	if !strings.Contains(got, "not initialized") {
		t.Errorf("output should report Git not initialized in a fresh temp dir:\n%s", got)
	}
	if !strings.Contains(got, "offline") {
		t.Errorf("output should mark GitHub/SonarQube as offline:\n%s", got)
	}
}
```

`filepath` and `strings` are already imported by `tests/integration/integration_test.go` (used throughout the existing `TestEndToEndGenerate*` tests) — no new imports needed.

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd tests && go test ./integration/... -run TestStatusOfflineShowsRepoAndGitOnly -v`
Expected: FAIL if Step 2/3 aren't both in place yet (either a build error from the CLI missing `status`, or a compile error in the test file); resolve any helper-name mismatch found in Step 3 before treating this as a true red step.

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd tests && go test ./integration/... -run TestStatusOfflineShowsRepoAndGitOnly -v`
Expected: PASS.

- [ ] **Step 6: Run the full test suite for `cli` and `tests` modules**

Run: `cd cli && go build ./... && go vet ./... && go test ./... && cd ../tests && go test ./...`
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add cli/main.go cli/internal/prompt/screens.go tests/integration/integration_test.go
git commit -m "feat: add lumo status command (repo/Git/GitHub/SonarQube)"
```

---

### Task 10: `lumo status` — dependency vulnerability row

**Files:**
- Modify: `cli/main.go` (extend `cmdStatus` with a fifth section, add `firstLine` helper)
- Modify: `tests/integration/integration_test.go` (add a sibling test to Task 9's)

**Interfaces:**
- Consumes: `cmdStatus` from Task 9 (appends to it, no new exported symbols).
- Produces: nothing consumed by later tasks — this is the plan's last task.

- [ ] **Step 1: Extend `cmdStatus` with the vulnerability-scan row**

Add to the end of `cmdStatus` (after the `SonarQube:` block from Task 9):

```go
	fmt.Println()
	fmt.Println(t.Header("Dependency vulnerabilities (Go):"))
	if _, statErr := os.Stat(filepath.Join(cwd, "go.mod")); statErr != nil {
		fmt.Println(t.Dim("skipped (no go.mod in current directory)"))
	} else if _, lookErr := exec.LookPath("go"); lookErr != nil {
		fmt.Println(t.Dim("skipped (go toolchain not found on PATH)"))
	} else {
		out, err := exec.Command("go", "run", "golang.org/x/vuln/cmd/govulncheck@latest", "./...").CombinedOutput()
		switch {
		case err != nil && !strings.Contains(string(out), "vulnerabilities"):
			fmt.Println(t.Dim("skipped (govulncheck unavailable: " + firstLine(string(out)) + ")"))
		case strings.Contains(string(out), "0 vulnerabilities"):
			fmt.Println(t.Success("no known vulnerabilities"))
		default:
			fmt.Println(t.Failure("vulnerabilities found — run 'go run golang.org/x/vuln/cmd/govulncheck@latest ./...' for details"))
		}
	}
```

Add a small helper below `printConnectorResult`:

```go
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
```

Add `"os/exec"` to `cli/main.go`'s import block if not already present (verify first — a duplicate import is a build error).

This follows `core/diag`'s best-effort/non-blocking philosophy (spec Section 2.4): a missing `go.mod`, missing `go` toolchain, or a `govulncheck` invocation failure all degrade to a "skipped" line, never a hard error that aborts the rest of `lumo status`. Non-Go projects (node/python/rust/etc. templates) are out of scope for this row — spec's explicit non-goal.

- [ ] **Step 2: Extend the integration test**

Add to `tests/integration/integration_test.go`, reusing the same `repoRoot`/`exeName`/`buildBinary` helpers as Task 9 Step 3:

```go
func TestStatusSkipsVulnRowOutsideGoProject(t *testing.T) {
	root := repoRoot(t)
	bin := t.TempDir()
	cliPath := filepath.Join(bin, exeName("lumo"))
	buildBinary(t, root, "cli", cliPath)

	dir := t.TempDir()
	cmd := exec.Command(cliPath, "status", "--offline")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lumo status --offline failed: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "skipped (no go.mod") {
		t.Errorf("output should skip the vuln row outside a Go project:\n%s", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they pass**

Run: `cd tests && go test ./integration/... -run TestStatus -v`
Expected: PASS for both `TestStatusOfflineShowsRepoAndGitOnly` and `TestStatusSkipsVulnRowOutsideGoProject`.

- [ ] **Step 4: Run the full repo test suite one last time**

Run: `cd cli && go build ./... && go vet ./... && go test ./... && cd ../core && go build ./... && go vet ./... && go test ./... && cd ../tests && go test ./...`
Expected: all pass, across `cli`, `core`, and `tests`.

- [ ] **Step 5: Commit**

```bash
git add cli/main.go tests/integration/integration_test.go
git commit -m "feat: add dependency vulnerability row to lumo status"
```
