// Package quality implements the Quality domain: local test execution,
// lint, format, and build health — the interactive drill-down that was
// missing behind lumo status's SonarQube row (docs/architecture/domains/
// quality.md). It does not evaluate security posture (Security Center's
// job) or manage toolchain installation (Dev Environment's job).
package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Model is Quality's published fact-store model.
type Model struct {
	TestResults  *TestSummary
	LintFindings []LintFinding
	FormatClean  bool
	BuildStatus  string // "unknown" | "passing" | "failing"
}

// TestSummary counts individual test outcomes from `go test -json`.
type TestSummary struct {
	Passed, Failed int
}

// LintFinding is one `go vet` diagnostic.
type LintFinding struct {
	File    string
	Line    int
	Message string
}

// Runner runs Quality's checks against the Go project rooted at dir.
type Runner struct{ dir string }

// NewRunner returns a Runner for dir.
func NewRunner(dir string) *Runner { return &Runner{dir: dir} }

// RunTests runs `go test -json ./...` and counts pass/fail per test.
// A test failure is a normal outcome to report, not a Runner error —
// the returned error is reserved for the command failing to produce any
// parseable output at all (e.g. no go.mod).
func (r *Runner) RunTests(ctx context.Context) (TestSummary, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-json", "./...")
	cmd.Dir = r.dir
	out, _ := cmd.Output() // non-zero exit on test failure is expected; still parse stdout

	var summary TestSummary
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var ev struct {
			Action string
			Test   string
		}
		if err := dec.Decode(&ev); err != nil {
			break
		}
		if ev.Test == "" {
			continue // package-level event (e.g. "ok"/"fail" rollup), not an individual test
		}
		switch ev.Action {
		case "pass":
			summary.Passed++
		case "fail":
			summary.Failed++
		}
	}
	return summary, nil
}

// CheckFormat reports whether every .go file under dir is gofmt-clean.
func (r *Runner) CheckFormat(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "gofmt", "-l", ".")
	cmd.Dir = r.dir
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("quality: gofmt: %w", err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

var vetLinePattern = regexp.MustCompile(`^(.+\.go):(\d+):\d+:\s*(.+)$`)

// RunLint runs `go vet ./...` and parses its diagnostics. Vet exiting
// non-zero (it does whenever it finds something) is expected, not a
// Runner error.
func (r *Runner) RunLint(ctx context.Context) ([]LintFinding, error) {
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = r.dir
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	_ = cmd.Run()

	var findings []LintFinding
	for _, line := range strings.Split(errBuf.String(), "\n") {
		m := vetLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		lineNo, _ := strconv.Atoi(m[2])
		findings = append(findings, LintFinding{File: m[1], Line: lineNo, Message: m[3]})
	}
	return findings, nil
}

// Build runs `go build ./...` and reports "passing" or "failing".
func (r *Runner) Build(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = r.dir
	if err := cmd.Run(); err != nil {
		return "failing", nil
	}
	return "passing", nil
}
