package workspaceintel

import (
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

func TestScan_EmptyDirDetectsNothing(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDetector().Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(m.Languages) != 0 {
		t.Errorf("Languages = %v, want none in an empty dir", m.Languages)
	}
	if len(m.ConfigFilesPresent) != 0 {
		t.Errorf("ConfigFilesPresent = %v, want none in an empty dir", m.ConfigFilesPresent)
	}
	if m.IsMonorepo {
		t.Error("IsMonorepo = true in an empty dir, want false")
	}
}

func TestScan_GoModDetectsGoLanguageAndVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/foo\n\ngo 1.25.0\n")

	m, err := NewDetector().Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !contains(m.Languages, "go") {
		t.Errorf("Languages = %v, want to include \"go\"", m.Languages)
	}
	if got := m.DeclaredToolchain["go"]; got != "1.25.0" {
		t.Errorf("DeclaredToolchain[\"go\"] = %q, want \"1.25.0\"", got)
	}
	if !contains(m.ConfigFilesPresent, "go.mod") {
		t.Errorf("ConfigFilesPresent = %v, want to include \"go.mod\"", m.ConfigFilesPresent)
	}
}

func TestScan_PackageJSONDetectsNodeEngine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "foo", "engines": {"node": ">=18"}}`)

	m, err := NewDetector().Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !contains(m.Languages, "javascript") {
		t.Errorf("Languages = %v, want to include \"javascript\"", m.Languages)
	}
	if got := m.DeclaredToolchain["node"]; got != ">=18" {
		t.Errorf("DeclaredToolchain[\"node\"] = %q, want \">=18\"", got)
	}
}

func TestScan_PackageJSONWithoutEnginesFieldIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "foo"}`)

	m, err := NewDetector().Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if _, ok := m.DeclaredToolchain["node"]; ok {
		t.Error("DeclaredToolchain should not have a \"node\" entry when engines.node is absent")
	}
}

func TestScan_GoWorkMarksMonorepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.work", "go 1.25.0\n\nuse (\n\t./cli\n\t./core\n)\n")

	m, err := NewDetector().Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !m.IsMonorepo {
		t.Error("IsMonorepo = false with a go.work file present, want true")
	}
}

func TestScan_DotGitIsRecordedAsConfigFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	m, err := NewDetector().Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !contains(m.ConfigFilesPresent, ".git") {
		t.Errorf("ConfigFilesPresent = %v, want to include \".git\"", m.ConfigFilesPresent)
	}
	// Workspace Intelligence records that .git exists; it does not
	// itself report branch/dirty state — that's Source Control's job.
	if len(m.Languages) != 0 {
		t.Errorf("Languages = %v, want none — .git alone implies no language", m.Languages)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
