// Package workspaceintel implements the Workspace Intelligence domain
// described in docs/architecture/domains/workspace-intelligence.md: it
// detects what a project *is* — declared languages, toolchain versions,
// monorepo layout, and which config files exist — so other domains (and
// lumo status, per ADR-0017's Phase B1 migration) read one shared model
// instead of each re-detecting the same facts inline.
//
// This package intentionally does not judge whether a detected toolchain
// version is adequate or installed on the machine (Dev Environment's
// job), and does not look at VCS state like branch or dirty status
// (Source Control's job, in the sibling core/domains/sourcecontrol
// package) — see workspace-intelligence.md's "Explicit non-goals".
package workspaceintel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Model is Workspace Intelligence's published fact-store model, matching
// the illustrative sketch in docs/architecture/domains/workspace-
// intelligence.md.
type Model struct {
	RootPath           string
	Languages          []string
	Frameworks         []string
	IsMonorepo         bool
	DeclaredToolchain  map[string]string // language -> version the project declares, e.g. {"go": "1.25"}
	ConfigFilesPresent []string          // relative paths, e.g. "go.mod", "go.work", "package.json", ".git"
	LastScanned        time.Time
}

// HasConfig reports whether name is in m.ConfigFilesPresent.
func (m Model) HasConfig(name string) bool {
	for _, c := range m.ConfigFilesPresent {
		if c == name {
			return true
		}
	}
	return false
}

// Detector scans a directory to produce a Model. It has no state of its
// own — every Scan is a fresh, independent detection pass.
type Detector struct{}

// NewDetector returns a Detector.
func NewDetector() Detector { return Detector{} }

// configCandidates are the files/directories Scan checks for at root's
// top level. Detection is deliberately shallow (top-level only) for this
// Phase B1 pass — recursive monorepo member detection is future work,
// not required by IsMonorepo's current go.work-presence heuristic.
var configCandidates = []string{"go.mod", "go.work", "package.json", ".git"}

// Scan detects languages, toolchain versions, and config files present
// at root. It never fails on a missing file (that just means the
// candidate isn't present) — the returned error is reserved for
// filesystem access failures such as a permission error stat-ing root
// itself.
func (Detector) Scan(root string) (Model, error) {
	m := Model{
		RootPath:          root,
		DeclaredToolchain: map[string]string{},
		LastScanned:       time.Now(),
	}

	for _, name := range configCandidates {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			m.ConfigFilesPresent = append(m.ConfigFilesPresent, name)
		}
	}

	if m.HasConfig("go.mod") {
		m.Languages = append(m.Languages, "go")
		if v, ok := goModVersion(filepath.Join(root, "go.mod")); ok {
			m.DeclaredToolchain["go"] = v
		}
	}
	if m.HasConfig("package.json") {
		m.Languages = append(m.Languages, "javascript")
		if v, ok := packageJSONNodeEngine(filepath.Join(root, "package.json")); ok {
			m.DeclaredToolchain["node"] = v
		}
	}
	if m.HasConfig("go.work") {
		m.IsMonorepo = true
	}

	return m, nil
}

var goModVersionPattern = regexp.MustCompile(`(?m)^go\s+(\S+)`)

// goModVersion extracts the "go X.Y" directive from a go.mod file. ok is
// false if the file can't be read or has no go directive.
func goModVersion(path string) (version string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	match := goModVersionPattern.FindSubmatch(data)
	if match == nil {
		return "", false
	}
	return string(match[1]), true
}

// packageJSONNodeEngine extracts package.json's "engines.node" field. ok
// is false if the file can't be read/parsed or declares no node engine.
func packageJSONNodeEngine(path string) (version string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var parsed struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", false
	}
	if parsed.Engines.Node == "" {
		return "", false
	}
	return parsed.Engines.Node, true
}
