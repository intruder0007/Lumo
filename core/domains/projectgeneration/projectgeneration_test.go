package projectgeneration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/intruder0007/Lumo/core/config"
	"github.com/intruder0007/Lumo/core/registry"
	sdk "github.com/intruder0007/Lumo/sdk/go/sdk"
)

func writePlugin(t *testing.T, dir, name, manifestJSON string) {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

const templateJSON = `{
  "protocolVersion": "1", "name": "go-rest-api", "version": "0.1.0", "kind": "template",
  "projectType": "backend-service", "language": "go", "framework": "rest-api",
  "entrypoint": "./t"
}`

const capabilityJSON = `{
  "protocolVersion": "1", "name": "readme", "version": "0.1.0", "kind": "capability",
  "capabilityId": "readme", "entrypoint": "./c"
}`

// fakeRunner mirrors core/engine's own test fake — Provider.Generate is
// a thin wrapper over engine.Engine.Run and shouldn't need its own
// plugin-subprocess test double logic.
type fakeRunner struct{}

func (fakeRunner) Generate(entrypointPath, expectedName, expectedProtocolVersion string, req sdk.GenerateRequest) (sdk.GenerateResponse, error) {
	return sdk.GenerateResponse{FilesWritten: []string{"go.mod", "main.go"}}, nil
}

func (fakeRunner) Apply(entrypointPath, expectedName, expectedProtocolVersion string, req sdk.ApplyRequest) (sdk.ApplyResponse, error) {
	return sdk.ApplyResponse{FilesWritten: []string{"README.md"}}, nil
}

func TestListTemplates_ReturnsDiscoveredTemplate(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "go-rest-api", templateJSON)

	p := NewProvider(registry.New(dir), fakeRunner{})
	templates, err := p.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].Name != "go-rest-api" {
		t.Fatalf("ListTemplates = %v, want one entry named go-rest-api", templates)
	}
}

func TestListCapabilities_ReturnsDiscoveredCapability(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "readme", capabilityJSON)

	p := NewProvider(registry.New(dir), fakeRunner{})
	caps, err := p.ListCapabilities()
	if err != nil {
		t.Fatalf("ListCapabilities: %v", err)
	}
	if len(caps) != 1 || caps[0].CapabilityID != "readme" {
		t.Fatalf("ListCapabilities = %v, want one entry with capabilityId readme", caps)
	}
}

func TestListTemplates_ExcludesCapabilities(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "go-rest-api", templateJSON)
	writePlugin(t, dir, "readme", capabilityJSON)

	p := NewProvider(registry.New(dir), fakeRunner{})
	templates, err := p.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("ListTemplates = %v, want exactly the one template (capability excluded)", templates)
	}
}

func TestModel_BeforeAnyGenerationHasNilLastGeneration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "go-rest-api", templateJSON)

	p := NewProvider(registry.New(dir), fakeRunner{})
	m, err := p.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	if m.LastGeneration != nil {
		t.Errorf("LastGeneration = %+v, want nil before any Generate call", m.LastGeneration)
	}
}

func TestGenerate_RecordsLastGenerationOnSuccess(t *testing.T) {
	pluginDir := t.TempDir()
	writePlugin(t, pluginDir, "go-rest-api", templateJSON)

	target := t.TempDir()
	p := NewProvider(registry.New(pluginDir), fakeRunner{})

	answers := config.Answers{
		ProjectName: "myapp",
		ProjectType: "backend-service",
		Language:    "go",
		Framework:   "rest-api",
	}
	summary, err := p.Generate(target, answers)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(summary.FilesWritten) == 0 {
		t.Error("Summary.FilesWritten is empty, want the fake runner's files")
	}

	m, err := p.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	if m.LastGeneration == nil || m.LastGeneration.TargetDir != target {
		t.Fatalf("LastGeneration = %+v, want TargetDir %q", m.LastGeneration, target)
	}
}

func TestGenerate_NoMatchingTemplateFails(t *testing.T) {
	dir := t.TempDir() // no plugins registered at all
	p := NewProvider(registry.New(dir), fakeRunner{})

	answers := config.Answers{
		ProjectName: "myapp",
		ProjectType: "backend-service",
		Language:    "go",
		Framework:   "rest-api",
	}
	if _, err := p.Generate(t.TempDir(), answers); err == nil {
		t.Fatal("Generate with no matching template should fail")
	}

	m, err := p.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	if m.LastGeneration != nil {
		t.Error("LastGeneration should stay nil after a failed Generate")
	}
}
