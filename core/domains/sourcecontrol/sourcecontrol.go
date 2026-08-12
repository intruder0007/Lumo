// Package sourcecontrol implements the Source Control domain: Git status,
// diff, and commit actions. It owns dynamic VCS state (branch, dirty,
// ahead/behind, remotes) — whether a directory is a git repo at all is a
// Workspace Intelligence fact (see core/domains/workspaceintel), not
// re-detected here.
package sourcecontrol

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Model is Source Control's published fact-store model.
type Model struct {
	Branch        string
	Dirty         bool
	Ahead, Behind int
	Remotes       []string
	WorktreeCount int
}

// NotARepositoryError is returned when dir is not inside a Git work tree.
type NotARepositoryError struct{ Dir string }

func (e *NotARepositoryError) Error() string {
	return fmt.Sprintf("sourcecontrol: %s is not a Git repository", e.Dir)
}

// ErrCommitNotConfirmed is returned by Commit when confirmed is false.
// Commit write actions are higher blast-radius than reading status, so
// the API forces a caller to pass an explicit confirmation rather than
// committing on a bare method call.
var ErrCommitNotConfirmed = errors.New("sourcecontrol: commit requires explicit confirmation")

// Provider reads and acts on the Git repository rooted at dir.
type Provider struct {
	dir string
	run func(args ...string) (stdout string, err error)
}

// NewProvider returns a Provider for the repository at dir.
func NewProvider(dir string) *Provider {
	return &Provider{
		dir: dir,
		run: func(args ...string) (string, error) {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			var out, errBuf bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errBuf
			if err := cmd.Run(); err != nil {
				if errBuf.Len() > 0 {
					return out.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(errBuf.String()))
				}
				return out.String(), err
			}
			return out.String(), nil
		},
	}
}

// IsRepo reports whether dir is inside a Git work tree.
func (p *Provider) IsRepo() bool {
	out, err := p.run("rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Status returns the current branch, dirty state, ahead/behind counts
// against the upstream (0/0 if there is none), configured remotes, and
// worktree count. It returns *NotARepositoryError if dir isn't a repo.
func (p *Provider) Status() (Model, error) {
	if !p.IsRepo() {
		return Model{}, &NotARepositoryError{Dir: p.dir}
	}

	var m Model

	if out, err := p.run("branch", "--show-current"); err == nil {
		m.Branch = strings.TrimSpace(out)
	}

	if out, err := p.run("status", "--porcelain"); err == nil {
		m.Dirty = strings.TrimSpace(out) != ""
	}

	if out, err := p.run("rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err == nil {
		if a, b, ok := parseAheadBehind(out); ok {
			m.Ahead, m.Behind = a, b
		}
	} // no upstream configured: Ahead/Behind stay 0, not an error

	if out, err := p.run("remote"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				m.Remotes = append(m.Remotes, line)
			}
		}
	}

	if out, err := p.run("worktree", "list", "--porcelain"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "worktree ") {
				m.WorktreeCount++
			}
		}
	}

	return m, nil
}

// parseAheadBehind parses the "A\tB" output of
// `git rev-list --left-right --count HEAD...@{upstream}`.
func parseAheadBehind(out string) (ahead, behind int, ok bool) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, false
	}
	a, errA := strconv.Atoi(fields[0])
	b, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		return 0, 0, false
	}
	return a, b, true
}

// Diff returns the diff against ref (e.g. "HEAD", a commit, a branch).
func (p *Provider) Diff(ref string) (string, error) {
	if !p.IsRepo() {
		return "", &NotARepositoryError{Dir: p.dir}
	}
	return p.run("diff", ref)
}

// Commit stages files and commits them with message. confirmed must be
// true — a bare call without it is refused (ErrCommitNotConfirmed) so a
// caller can't trigger a write action by accident.
func (p *Provider) Commit(message string, files []string, confirmed bool) error {
	if !confirmed {
		return ErrCommitNotConfirmed
	}
	if !p.IsRepo() {
		return &NotARepositoryError{Dir: p.dir}
	}
	if len(files) > 0 {
		if _, err := p.run(append([]string{"add"}, files...)...); err != nil {
			return fmt.Errorf("sourcecontrol: git add: %w", err)
		}
	}
	if _, err := p.run("commit", "-m", message); err != nil {
		return fmt.Errorf("sourcecontrol: git commit: %w", err)
	}
	return nil
}
