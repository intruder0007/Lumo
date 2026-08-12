package quality

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func newFixtureModule(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH")
	}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/fixture\n\ngo 1.25\n")
	return dir
}

func TestRunTests_AllPassing(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "pass_test.go", `package fixture

import "testing"

func TestOK(t *testing.T) {}
`)
	summary, err := NewRunner(dir).RunTests(context.Background())
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if summary.Passed != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want {Passed: 1, Failed: 0}", summary)
	}
}

func TestRunTests_CountsFailures(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "fail_test.go", `package fixture

import "testing"

func TestGood(t *testing.T) {}
func TestBad(t *testing.T) { t.Fail() }
`)
	summary, err := NewRunner(dir).RunTests(context.Background())
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if summary.Passed != 1 || summary.Failed != 1 {
		t.Fatalf("summary = %+v, want {Passed: 1, Failed: 1}", summary)
	}
}

func TestCheckFormat_CleanFile(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "clean.go", "package fixture\n\nfunc F() {}\n")

	clean, err := NewRunner(dir).CheckFormat(context.Background())
	if err != nil {
		t.Fatalf("CheckFormat: %v", err)
	}
	if !clean {
		t.Error("CheckFormat = false for a gofmt-clean file, want true")
	}
}

func TestCheckFormat_UnformattedFile(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "messy.go", "package fixture\nfunc   F( ) {\n}\n")

	clean, err := NewRunner(dir).CheckFormat(context.Background())
	if err != nil {
		t.Fatalf("CheckFormat: %v", err)
	}
	if clean {
		t.Error("CheckFormat = true for a badly-formatted file, want false")
	}
}

func TestRunLint_FindsVetIssue(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "vet.go", `package fixture

import "fmt"

func F() {
	fmt.Printf("%d\n", "not a number")
}
`)
	findings, err := NewRunner(dir).RunLint(context.Background())
	if err != nil {
		t.Fatalf("RunLint: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("RunLint found no issues in code with a Printf format mismatch")
	}
	if findings[0].File == "" || findings[0].Line == 0 {
		t.Errorf("finding = %+v, want a non-empty file and non-zero line", findings[0])
	}
}

func TestRunLint_CleanCodeHasNoFindings(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "clean.go", "package fixture\n\nfunc F() {}\n")

	findings, err := NewRunner(dir).RunLint(context.Background())
	if err != nil {
		t.Fatalf("RunLint: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestBuild_ValidPackagePasses(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "main.go", "package fixture\n\nfunc F() {}\n")

	status, err := NewRunner(dir).Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if status != "passing" {
		t.Errorf("BuildStatus = %q, want \"passing\"", status)
	}
}

func TestBuild_BrokenPackageFails(t *testing.T) {
	dir := newFixtureModule(t)
	writeFile(t, dir, "broken.go", "package fixture\n\nfunc F( {\n")

	status, err := NewRunner(dir).Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if status != "failing" {
		t.Errorf("BuildStatus = %q, want \"failing\"", status)
	}
}
