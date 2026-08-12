package securitycenter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func scannerWithFakeVulnCheck(dir string, out string, err error) *Scanner {
	s := NewScanner(dir)
	s.runVulnCheck = func(context.Context) (string, error) { return out, err }
	return s
}

func TestScan_NoGoModSkipsVulnCheck(t *testing.T) {
	dir := t.TempDir()
	called := false
	s := NewScanner(dir)
	s.runVulnCheck = func(context.Context) (string, error) {
		called = true
		return "", nil
	}

	m, err := s.Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if called {
		t.Error("runVulnCheck was called with no go.mod present")
	}
	if m.GateState != "pass" {
		t.Errorf("GateState = %q, want \"pass\"", m.GateState)
	}
}

func TestScan_CleanGovulncheckOutputYieldsNoVulnerabilities(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/foo\n\ngo 1.25.0\n")

	s := scannerWithFakeVulnCheck(dir, "No vulnerabilities found.\n", nil)
	m, err := s.Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(m.Vulnerabilities) != 0 {
		t.Errorf("Vulnerabilities = %v, want none", m.Vulnerabilities)
	}
	if m.GateState != "pass" {
		t.Errorf("GateState = %q, want \"pass\"", m.GateState)
	}
}

func TestScan_VulnerableOutputExtractsIDsAndBlocks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/foo\n\ngo 1.25.0\n")

	canned := "Vulnerability #1: GO-2026-1234\n  More info: https://pkg.go.dev/vuln/GO-2026-1234\nVulnerability #2: GO-2026-5678\n"
	s := scannerWithFakeVulnCheck(dir, canned, nil)
	m, err := s.Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(m.Vulnerabilities) != 2 {
		t.Fatalf("Vulnerabilities = %v, want 2 unique IDs", m.Vulnerabilities)
	}
	if m.GateState != "block" {
		t.Errorf("GateState = %q, want \"block\"", m.GateState)
	}
}

func TestScan_DetectsAWSKeyInFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.env", "AWS_KEY=AKIAABCDEFGHIJKLMNOP\n")

	m, err := NewScanner(dir).Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(m.SecretsFound) != 1 || m.SecretsFound[0].Path != "config.env" {
		t.Fatalf("SecretsFound = %v, want one finding in config.env", m.SecretsFound)
	}
	if m.GateState != "block" {
		t.Errorf("GateState = %q, want \"block\" with a secret present", m.GateState)
	}
}

func TestScan_CleanDirHasNoSecretsAndPasses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "just docs, nothing sensitive\n")

	m, err := NewScanner(dir).Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(m.SecretsFound) != 0 {
		t.Errorf("SecretsFound = %v, want none", m.SecretsFound)
	}
	if m.GateState != "pass" {
		t.Errorf("GateState = %q, want \"pass\"", m.GateState)
	}
}

func TestScan_SkipsGitDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".git"), "config", "-----BEGIN RSA PRIVATE KEY-----\n")

	m, err := NewScanner(dir).Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(m.SecretsFound) != 0 {
		t.Errorf("SecretsFound = %v, want none — .git contents must be skipped", m.SecretsFound)
	}
}

func TestScan_PluginTrustIsHonestStub(t *testing.T) {
	dir := t.TempDir()
	m, err := NewScanner(dir).Scan(context.Background(), []string{"git-init", "readme"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, name := range []string{"git-init", "readme"} {
		trust, ok := m.PluginTrust[name]
		if !ok {
			t.Fatalf("PluginTrust missing entry for %q", name)
		}
		if trust.Signed || trust.Verified {
			t.Errorf("PluginTrust[%q] = %+v, want {false, false} until signing work lands", name, trust)
		}
	}
}
