package devenvironment

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiagnose_KnownInstalledToolIsReported(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found on PATH")
	}
	d := NewDoctor()
	m, err := d.Diagnose(map[string]string{"go": "1.25.0"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if _, ok := m.InstalledToolVersions["go"]; !ok {
		t.Errorf("InstalledToolVersions = %v, want a \"go\" entry", m.InstalledToolVersions)
	}
	if len(m.MissingTools) != 0 {
		t.Errorf("MissingTools = %v, want none", m.MissingTools)
	}
}

func TestDiagnose_UnmappedLanguageIsSkippedNotMissing(t *testing.T) {
	d := NewDoctor()
	m, err := d.Diagnose(map[string]string{"cobol": "1985"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(m.MissingTools) != 0 {
		t.Errorf("MissingTools = %v, want none for a language this package doesn't know how to check", m.MissingTools)
	}
}

func TestDiagnose_MissingToolIsReported(t *testing.T) {
	t.Setenv("PATH", "")
	d := NewDoctor()
	m, err := d.Diagnose(map[string]string{"go": "1.25.0"})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(m.MissingTools) != 1 || m.MissingTools[0] != "go" {
		t.Errorf("MissingTools = %v, want [\"go\"] with an empty PATH", m.MissingTools)
	}
}

func TestDiagnose_MissingDirectoryIsFlaggedWithRemediation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist-yet")
	d := NewDoctor(dir)

	m, err := d.Diagnose(nil)
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(m.DiagnosticIssues) != 1 {
		t.Fatalf("DiagnosticIssues = %v, want exactly one", m.DiagnosticIssues)
	}
	wantID := "missing-dir:" + dir
	if m.DiagnosticIssues[0].ID != wantID {
		t.Errorf("issue ID = %q, want %q", m.DiagnosticIssues[0].ID, wantID)
	}
	if len(m.RemediationsAvailable) != 1 || m.RemediationsAvailable[0] != wantID {
		t.Errorf("RemediationsAvailable = %v, want [%q]", m.RemediationsAvailable, wantID)
	}
}

func TestRemediateThenDiagnose_IssueGoesAway(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "should-be-created")
	d := NewDoctor(dir)

	before, _ := d.Diagnose(nil)
	if len(before.DiagnosticIssues) != 1 {
		t.Fatalf("expected the directory to be missing before Remediate, got %v", before.DiagnosticIssues)
	}

	if err := d.Remediate(before.DiagnosticIssues[0].ID); err != nil {
		t.Fatalf("Remediate: %v", err)
	}

	after, _ := d.Diagnose(nil)
	if len(after.DiagnosticIssues) != 0 {
		t.Errorf("DiagnosticIssues after Remediate = %v, want none", after.DiagnosticIssues)
	}
}

func TestRemediate_UnknownIssueReturnsError(t *testing.T) {
	d := NewDoctor()
	if err := d.Remediate("no-such-issue"); err == nil {
		t.Fatal("Remediate on an unknown issue ID should return an error")
	}
}
