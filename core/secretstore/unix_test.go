package secretstore

import (
	"slices"
	"testing"
)

type fakeRunner struct {
	calls  [][]string
	stdout string
	err    error
}

func (f *fakeRunner) Run(name string, args []string, stdin string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.stdout, f.err
}

func TestMacStoreSetBuildsExpectedCommand(t *testing.T) {
	r := &fakeRunner{}
	s := &macStore{run: r}
	if err := s.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"security", "add-generic-password", "-a", "lumo", "-s", "sonarqube-token", "-w", "abc123", "-U"}
	if len(r.calls) != 1 || !slices.Equal(r.calls[0], want) {
		t.Errorf("got calls %v, want one call %v", r.calls, want)
	}
}

func TestMacStoreGetNotFoundIsNotAnError(t *testing.T) {
	r := &fakeRunner{err: errExitStatus44}
	s := &macStore{run: r}
	secret, found, err := s.Get("sonarqube-token")
	if err != nil || found || secret != "" {
		t.Errorf("Get on missing key: secret=%q found=%v err=%v, want secret=\"\" found=false err=nil", secret, found, err)
	}
}

func TestLinuxStoreSetPassesSecretOnStdin(t *testing.T) {
	r := &fakeRunner{}
	s := &linuxStore{run: r}
	if err := s.Set("sonarqube-token", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	wantArgs := []string{"secret-tool", "store", "--label", "Lumo: sonarqube-token", "service", "lumo", "key", "sonarqube-token"}
	if len(r.calls) != 1 || !slices.Equal(r.calls[0], wantArgs) {
		t.Errorf("got calls %v, want one call %v", r.calls, wantArgs)
	}
}

func TestLinuxStoreGetNotFoundIsNotAnError(t *testing.T) {
	r := &fakeRunner{err: errExitStatus44}
	s := &linuxStore{run: r}
	secret, found, err := s.Get("sonarqube-token")
	if err != nil || found || secret != "" {
		t.Errorf("Get on missing key: secret=%q found=%v err=%v, want secret=\"\" found=false err=nil", secret, found, err)
	}
}
