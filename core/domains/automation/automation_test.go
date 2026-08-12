package automation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestListTasks_ReturnsRegisteredTasksSortedByID(t *testing.T) {
	r := NewRunner(t.TempDir())
	r.RegisterTask(TaskDef{ID: "b-task", Command: []string{"true"}})
	r.RegisterTask(TaskDef{ID: "a-task", Command: []string{"true"}})

	got := r.ListTasks()
	if len(got) != 2 || got[0].ID != "a-task" || got[1].ID != "b-task" {
		t.Fatalf("ListTasks = %v, want [a-task, b-task]", got)
	}
}

func TestRunTask_UnknownIDFails(t *testing.T) {
	r := NewRunner(t.TempDir())
	_, err := r.RunTask(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrUnknownTask) {
		t.Fatalf("RunTask(unknown) = %v, want ErrUnknownTask", err)
	}
}

func TestRunTask_SuccessfulCommand(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found on PATH")
	}
	r := NewRunner(t.TempDir())
	r.RegisterTask(TaskDef{ID: "go-version", Command: []string{"go", "version"}})

	result, err := r.RunTask(context.Background(), "go-version")
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Output == "" {
		t.Error("Output is empty, want the go version banner")
	}
}

func TestRunTask_NonZeroExitIsNotAnError(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found on PATH")
	}
	r := NewRunner(t.TempDir())
	// An unrecognized `go` subcommand reliably exits non-zero without
	// needing a platform-specific failing binary.
	r.RegisterTask(TaskDef{ID: "bad-subcommand", Command: []string{"go", "not-a-real-subcommand"}})

	result, err := r.RunTask(context.Background(), "bad-subcommand")
	if err != nil {
		t.Fatalf("RunTask returned an error for a normal non-zero exit: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("ExitCode = 0, want non-zero for an unrecognized go subcommand")
	}
}

// TestRunTask_ProjectTestCommandEndToEnd is the concrete Phase B3 slice:
// "run the project's test command from the TUI" — here, running it
// through the Automation domain end to end against a real fixture
// module, before any TUI wiring exists.
func TestRunTask_ProjectTestCommandEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}
	testFile := `package fixture

import "testing"

func TestOK(t *testing.T) {}
`
	if err := os.WriteFile(filepath.Join(dir, "ok_test.go"), []byte(testFile), 0o644); err != nil {
		t.Fatalf("writing ok_test.go: %v", err)
	}

	r := NewRunner(dir)
	r.RegisterTask(TaskDef{ID: "test", Command: []string{"go", "test", "./..."}})

	result, err := r.RunTask(context.Background(), "test")
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 for a passing test suite:\n%s", result.ExitCode, result.Output)
	}
}
