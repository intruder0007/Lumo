// Package automation implements the Automation domain: running a
// registered task and reporting its exit status and output. Per
// docs/architecture/v2-platform-architecture.md's Phase B3 plan, this
// first slice is deliberately narrow (synchronous, single task run) —
// hooks, scheduling, and a running-jobs list are future increments, not
// built ahead of a concrete need (Automation is the domain with no V1
// precedent and the highest over-design risk in Phase B).
package automation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
)

// TaskDef is one runnable task: an id and the argv to execute.
type TaskDef struct {
	ID      string
	Command []string // argv, e.g. []string{"go", "test", "./..."}
}

// JobResult is the outcome of one RunTask call. Automation reports that
// a task ran and its exit status; it does not parse or own the
// task's output — a task invoking Quality's test runner, for instance,
// still leaves the detailed pass/fail model to Quality.
type JobResult struct {
	TaskID   string
	ExitCode int
	Output   string
}

// ErrUnknownTask is returned by RunTask for an unregistered task ID.
var ErrUnknownTask = errors.New("automation: unknown task")

// Runner runs registered tasks in dir.
type Runner struct {
	dir   string
	tasks map[string]TaskDef
}

// NewRunner returns an empty Runner rooted at dir.
func NewRunner(dir string) *Runner {
	return &Runner{dir: dir, tasks: make(map[string]TaskDef)}
}

// RegisterTask adds or replaces a task definition.
func (r *Runner) RegisterTask(t TaskDef) {
	r.tasks[t.ID] = t
}

// ListTasks returns all registered tasks, ordered by ID for a stable,
// testable result.
func (r *Runner) ListTasks() []TaskDef {
	ids := make([]string, 0, len(r.tasks))
	for id := range r.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]TaskDef, len(ids))
	for i, id := range ids {
		out[i] = r.tasks[id]
	}
	return out
}

// RunTask runs the task registered under id and blocks until it exits.
// A non-zero exit code is a normal JobResult, not a Runner error — the
// error return is reserved for the task ID being unknown or the command
// failing to start at all.
func (r *Runner) RunTask(ctx context.Context, id string) (JobResult, error) {
	def, ok := r.tasks[id]
	if !ok {
		return JobResult{}, fmt.Errorf("%w: %q", ErrUnknownTask, id)
	}
	if len(def.Command) == 0 {
		return JobResult{}, fmt.Errorf("automation: task %q has no command", id)
	}

	cmd := exec.CommandContext(ctx, def.Command[0], def.Command[1:]...)
	cmd.Dir = r.dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	result := JobResult{TaskID: id, Output: buf.String()}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("automation: running task %q: %w", id, err)
}
