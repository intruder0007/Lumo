// core/connector/github_test.go
package connector

import (
	"errors"
	"testing"

	"github.com/intruder0007/Lumo/core/diag"
)

type fakeGHRunner struct {
	responses map[string]struct {
		stdout string
		err    error
	}
}

func (f fakeGHRunner) Run(name string, args []string) (string, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	r, ok := f.responses[key]
	if !ok {
		return "", errors.New("unexpected command: " + key)
	}
	return r.stdout, r.err
}

func TestGitHubConnectorReportsConnectedWhenAuthenticated(t *testing.T) {
	run := fakeGHRunner{responses: map[string]struct {
		stdout string
		err    error
	}{
		"gh auth status": {stdout: "Logged in to github.com as octocat", err: nil},
	}}
	c := NewGitHubConnector(run)
	res, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if !res.Connected {
		t.Errorf("Connected = false, want true")
	}
	if res.Detail == "" {
		t.Errorf("Detail is empty, want the account name surfaced")
	}
}

func TestGitHubConnectorReportsNotConnectedWhenGhMissing(t *testing.T) {
	run := fakeGHRunner{responses: map[string]struct {
		stdout string
		err    error
	}{}}
	c := NewGitHubConnector(run)
	res, err := RunEngine(c, diag.NoopLogger{})
	if err != nil {
		t.Fatalf("RunEngine: %v", err)
	}
	if res.Connected {
		t.Errorf("Connected = true, want false when gh is unavailable")
	}
}
