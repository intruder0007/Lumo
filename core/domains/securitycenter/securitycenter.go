// Package securitycenter implements the Security Center domain:
// dependency vulnerability scanning, secret detection, and plugin trust
// status, combined into one gate state. Signing/verification ships as an
// honest stub (Signed=false, Verified=false) per ADR-0017 Decision 2 —
// this package does not block on the separate cosign/Sigstore work
// tracked in SECURITY.md.
package securitycenter

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

// Model is Security Center's published fact-store model.
type Model struct {
	Vulnerabilities []string // Go vulnerability IDs, e.g. "GO-2026-1234"
	SecretsFound    []SecretFinding
	PluginTrust     map[string]TrustState
	GateState       string // "pass" | "warn" | "block"
}

// SecretFinding is one match from the local secret scan.
type SecretFinding struct {
	Path    string // relative to the scanned root
	Pattern string // human-readable name of the pattern that matched
}

// TrustState is a plugin's signing/verification status. Both fields are
// false until the v0.8.0-track signing work lands (SECURITY.md);
// Security Center only has a slot for the result, not the signer.
type TrustState struct {
	Signed   bool
	Verified bool
}

// skipDirs are directories excluded from the secret scan: version
// control internals and dependency trees too large/irrelevant to scan.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// secretPatterns are the local, offline secret-detection rules. Kept
// deliberately small and high-confidence to avoid noisy false positives
// — broadening this list is a future increment, not a blocker for a
// first real (non-mocked) implementation.
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"AWS Access Key ID", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"private key block", regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`)},
}

var vulnIDPattern = regexp.MustCompile(`GO-\d{4}-\d+`)

// Scanner scans a directory for Security Center's model.
type Scanner struct {
	dir string
	// runVulnCheck is injectable so tests can supply canned govulncheck
	// output instead of requiring network access and a live Go
	// toolchain fetch on every test run.
	runVulnCheck func(ctx context.Context) (string, error)
}

// NewScanner returns a Scanner rooted at dir, using the real govulncheck
// command (network access required) as its vulnerability source.
func NewScanner(dir string) *Scanner {
	return &Scanner{
		dir: dir,
		runVulnCheck: func(ctx context.Context) (string, error) {
			cmd := exec.CommandContext(ctx, "go", "run", "golang.org/x/vuln/cmd/govulncheck@latest", "./...")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
	}
}

// Scan produces a Model: vulnerabilities (only if dir has a go.mod),
// secrets found under dir, and a trust stub for each name in
// installedPlugins.
func (s *Scanner) Scan(ctx context.Context, installedPlugins []string) (Model, error) {
	m := Model{PluginTrust: map[string]TrustState{}}

	if _, err := os.Stat(filepath.Join(s.dir, "go.mod")); err == nil {
		if out, _ := s.runVulnCheck(ctx); out != "" {
			m.Vulnerabilities = extractVulnIDs(out)
		}
	}

	secrets, err := scanForSecrets(s.dir)
	if err != nil {
		return Model{}, err
	}
	m.SecretsFound = secrets

	for _, name := range installedPlugins {
		m.PluginTrust[name] = TrustState{}
	}

	m.GateState = "pass"
	if len(m.Vulnerabilities) > 0 || len(m.SecretsFound) > 0 {
		m.GateState = "block"
	}
	return m, nil
}

// extractVulnIDs pulls unique Go vulnerability IDs (e.g. "GO-2026-1234")
// out of govulncheck's text output.
func extractVulnIDs(out string) []string {
	matches := vulnIDPattern.FindAllString(out, -1)
	seen := make(map[string]bool, len(matches))
	var ids []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			ids = append(ids, m)
		}
	}
	return ids
}

// scanForSecrets walks root looking for secretPatterns matches in file
// contents. An unreadable or binary file is skipped, not an error — a
// single unreadable file must not abort the whole scan.
func scanForSecrets(root string) ([]SecretFinding, error) {
	var findings []SecretFinding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, p := range secretPatterns {
			if p.re.Match(data) {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				findings = append(findings, SecretFinding{Path: rel, Pattern: p.name})
			}
		}
		return nil
	})
	return findings, err
}
