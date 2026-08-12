// Package devenvironment implements the Dev Environment domain: whether
// the machine has the toolchains a project declares, plus the
// diagnose-then-fix directory checks that used to be `lumo doctor`'s
// whole job. It does not scan for vulnerabilities (Security Center) or
// run a project's own test suite (Quality) — see docs/architecture/
// domains/dev-environment.md.
package devenvironment

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Model is Dev Environment's published fact-store model.
type Model struct {
	InstalledToolVersions map[string]string // language -> raw version string found on the machine
	MissingTools          []string          // languages declared by the project with no tool found on PATH
	DiagnosticIssues      []DoctorIssue
	RemediationsAvailable []string // DoctorIssue.ID values Remediate can fix
}

// DoctorIssue is one diagnostic finding.
type DoctorIssue struct {
	ID      string
	Message string
}

// toolForLanguage maps a workspaceintel-declared language to the binary
// that provides its toolchain. A language with no entry here is skipped
// by Diagnose, not reported missing — this package only judges what it
// knows how to check.
var toolForLanguage = map[string]string{
	"go":         "go",
	"javascript": "node",
}

// Doctor diagnoses and remediates a machine's readiness for a project.
// dirs are directories expected to exist (the same discovery-directory
// check `lumo doctor` already performed).
type Doctor struct {
	dirs []string
}

// NewDoctor returns a Doctor that also checks dirs for existence.
func NewDoctor(dirs ...string) *Doctor {
	return &Doctor{dirs: dirs}
}

// Diagnose compares declaredToolchain (from a workspaceintel.Model) against
// what's actually installed, and checks that every configured directory
// exists.
func (d *Doctor) Diagnose(declaredToolchain map[string]string) (Model, error) {
	m := Model{InstalledToolVersions: map[string]string{}}

	for lang := range declaredToolchain {
		tool, known := toolForLanguage[lang]
		if !known {
			continue
		}
		if _, err := exec.LookPath(tool); err != nil {
			m.MissingTools = append(m.MissingTools, lang)
			continue
		}
		if version, err := detectVersion(tool); err == nil {
			m.InstalledToolVersions[lang] = version
		}
	}

	for _, dir := range d.dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			continue
		}
		id := "missing-dir:" + dir
		m.DiagnosticIssues = append(m.DiagnosticIssues, DoctorIssue{
			ID:      id,
			Message: "expected directory not found: " + dir,
		})
		m.RemediationsAvailable = append(m.RemediationsAvailable, id)
	}

	return m, nil
}

// detectVersion runs tool's version command and returns the raw,
// trimmed output.
func detectVersion(tool string) (string, error) {
	var cmd *exec.Cmd
	if tool == "go" {
		cmd = exec.Command("go", "version")
	} else {
		cmd = exec.Command(tool, "--version")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Remediate fixes issueID if a remediation is known for it. Today the
// only known remediation is creating a missing directory — this is a
// first real, narrow slice per the Phase B3 plan, not a general fixer.
func (d *Doctor) Remediate(issueID string) error {
	const prefix = "missing-dir:"
	if !strings.HasPrefix(issueID, prefix) {
		return fmt.Errorf("devenvironment: no remediation available for %q", issueID)
	}
	dir := strings.TrimPrefix(issueID, prefix)
	return os.MkdirAll(dir, 0o755)
}
