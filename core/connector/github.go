// core/connector/github.go
package connector

import (
	"os/exec"
	"strings"
)

// cmdRunner abstracts exec.Command so GitHubConnector is testable without
// the real gh binary (see fakeGHRunner in github_test.go). The real
// implementation is ExecCmdRunner, constructed by callers (Task 9's
// cmdStatus) rather than by this package, so core/connector's exported
// surface stays free of os/exec side effects at import time.
type cmdRunner interface {
	Run(name string, args []string) (stdout string, err error)
}

// ExecCmdRunner is the production cmdRunner, shelling out via os/exec.
type ExecCmdRunner struct{}

func (ExecCmdRunner) Run(name string, args []string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// GitHubConnector reports whether the `gh` CLI is authenticated. No
// GitHub token is ever handled by Lumo directly — this reuses whatever
// session `gh auth login` already established. If `gh` isn't installed or
// isn't authenticated, the connector reports "not connected", never an
// error (see the auth phase below).
type GitHubConnector struct {
	run cmdRunner
}

func NewGitHubConnector(run cmdRunner) *GitHubConnector {
	return &GitHubConnector{run: run}
}

func (c *GitHubConnector) Name() string { return "github" }

func (c *GitHubConnector) Phases() []Phase {
	return []Phase{
		{Name: "auth", Run: c.auth},
	}
}

func (c *GitHubConnector) auth() (Result, error) {
	out, err := c.run.Run("gh", []string{"auth", "status"})
	if err != nil {
		return Result{Connected: false, Detail: "gh not authenticated (or not installed)"}, nil
	}
	return Result{Connected: true, Detail: strings.TrimSpace(out)}, nil
}
