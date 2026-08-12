package shell

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/intruder0007/Lumo/core/kernel"
)

// repoRoot walks up from this test file to the repository root (three
// levels above cli/internal/shell), so these tests exercise a real
// project instead of a synthetic fixture.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func TestNewLiveShell_WorkspaceIntelligenceSeesGoModule(t *testing.T) {
	root := repoRoot(t)
	s := NewLiveShell(root, kernel.NewSessionStore(), nil)

	if err := s.Focus(DomainWorkspaceIntelligence); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	_, panel, _ := s.Current()
	content := panel.Render()
	if !strings.Contains(content, "Languages:   go\n") {
		t.Errorf("Workspace panel = %q, want a \"Languages:   go\" line (this repo's root has go.work, no root go.mod)", content)
	}
}

func TestNewLiveShell_SourceControlSeesGitRepo(t *testing.T) {
	root := repoRoot(t)
	s := NewLiveShell(root, kernel.NewSessionStore(), nil)

	if err := s.Focus(DomainSourceControl); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	_, panel, _ := s.Current()
	content := panel.Render()
	if strings.Contains(content, "not a Git repository") {
		t.Errorf("Source Control panel = %q, want real branch data (this repo is a git repo)", content)
	}
	if !strings.Contains(content, "Branch:") {
		t.Errorf("Source Control panel = %q, want a Branch: line", content)
	}
}

func TestNewLiveShell_DevEnvironmentSeesGoToolchain(t *testing.T) {
	root := repoRoot(t)
	s := NewLiveShell(root, kernel.NewSessionStore(), nil)

	if err := s.Focus(DomainDevEnvironment); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	_, panel, _ := s.Current()
	content := panel.Render()
	if !strings.Contains(content, "go:") {
		t.Errorf("Dev Environment panel = %q, want an installed go: entry (go is required to run this test)", content)
	}
}

func TestNewLiveShell_QualityAndAutomationStayPlaceholders(t *testing.T) {
	root := repoRoot(t)
	s := NewLiveShell(root, kernel.NewSessionStore(), nil)

	if err := s.Focus(DomainQuality); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	_, panel, _ := s.Current()
	if !strings.Contains(panel.Render(), "on demand") {
		t.Error("Quality panel should stay a placeholder — no eager test/build/vet run at shell startup")
	}

	if err := s.Focus(DomainAutomation); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	_, panel, _ = s.Current()
	if !strings.Contains(panel.Render(), "no tasks registered") {
		t.Error("Automation panel should say no tasks are registered yet")
	}
}

func TestNewLiveShell_StillRegistersAllEightDomains(t *testing.T) {
	root := repoRoot(t)
	s := NewLiveShell(root, kernel.NewSessionStore(), nil)

	seen := map[string]bool{}
	start, _, ok := s.Current()
	if !ok {
		t.Fatal("NewLiveShell should start with a focused panel")
	}
	domain := start
	for i := 0; i < 8; i++ {
		seen[string(domain)] = true
		s.Next()
		domain, _, _ = s.Current()
	}
	if len(seen) != 8 {
		t.Fatalf("visited %d distinct domains, want 8: %v", len(seen), seen)
	}
}
