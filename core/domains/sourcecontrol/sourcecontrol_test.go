package sourcecontrol

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a git repo in a temp dir with a committed file on
// a known branch, using a local, isolated identity so the test doesn't
// depend on the host's global git config.
func initTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")
	return dir
}

func TestStatus_NotARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	p := NewProvider(t.TempDir())
	_, err := p.Status()
	var want *NotARepositoryError
	if !errors.As(err, &want) {
		t.Fatalf("Status on a non-repo = %v, want *NotARepositoryError", err)
	}
}

func TestStatus_CleanRepoOnMain(t *testing.T) {
	dir := initTestRepo(t)
	m, err := NewProvider(dir).Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if m.Branch != "main" {
		t.Errorf("Branch = %q, want \"main\"", m.Branch)
	}
	if m.Dirty {
		t.Error("Dirty = true right after a commit, want false")
	}
	if m.Ahead != 0 || m.Behind != 0 {
		t.Errorf("Ahead/Behind = %d/%d, want 0/0 with no upstream", m.Ahead, m.Behind)
	}
}

func TestStatus_DirtyAfterUncommittedChange(t *testing.T) {
	dir := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}

	m, err := NewProvider(dir).Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !m.Dirty {
		t.Error("Dirty = false after modifying a tracked file, want true")
	}
}

func TestCommit_RefusesWithoutConfirmation(t *testing.T) {
	dir := initTestRepo(t)
	err := NewProvider(dir).Commit("should not happen", nil, false)
	if !errors.Is(err, ErrCommitNotConfirmed) {
		t.Fatalf("Commit without confirmation = %v, want ErrCommitNotConfirmed", err)
	}
}

func TestCommit_ConfirmedCommitsStagedFile(t *testing.T) {
	dir := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("writing new.txt: %v", err)
	}

	p := NewProvider(dir)
	if err := p.Commit("add new.txt", []string{"new.txt"}, true); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	m, err := p.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if m.Dirty {
		t.Error("Dirty = true after committing all changes, want false")
	}
}

func TestDiff_NotARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
	p := NewProvider(t.TempDir())
	_, err := p.Diff("HEAD")
	var want *NotARepositoryError
	if !errors.As(err, &want) {
		t.Fatalf("Diff on a non-repo = %v, want *NotARepositoryError", err)
	}
}

func TestDiff_ShowsUncommittedChange(t *testing.T) {
	dir := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}

	out, err := NewProvider(dir).Diff("HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if out == "" {
		t.Error("Diff returned empty output for a modified tracked file")
	}
}
