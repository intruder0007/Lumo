// live.go wires the eight stub panels (domains.go) to real domain data,
// per Phase B4 of docs/architecture/v2-platform-architecture.md.
//
// Not every domain scans live on shell startup: Quality's checks (full
// test run, build, vet) and Automation's tasks are too slow/undefined
// to run eagerly against an arbitrary project, so those two stay
// informative placeholders here — a deliberate scope decision, not an
// oversight, matching the "don't over-design ahead of a concrete need"
// principle used throughout Phase B. Security Center's dependency-
// vulnerability check is network-bound (govulncheck), so it runs under
// a short timeout rather than blocking shell startup; its secret scan
// (local, fast) always runs.
package shell

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/intruder0007/Lumo/core/domains/devenvironment"
	"github.com/intruder0007/Lumo/core/domains/projectgeneration"
	"github.com/intruder0007/Lumo/core/domains/securitycenter"
	"github.com/intruder0007/Lumo/core/domains/sourcecontrol"
	"github.com/intruder0007/Lumo/core/domains/workspaceintel"
	"github.com/intruder0007/Lumo/core/kernel"
	"github.com/intruder0007/Lumo/core/plugin"
	"github.com/intruder0007/Lumo/core/registry"
)

// vulnScanTimeout bounds Security Center's network-bound govulncheck
// call so opening the shell can never hang on a slow/offline network.
const vulnScanTimeout = 3 * time.Second

// NewLiveShell returns a Shell like NewPlatformShell, but with each
// scannable domain's panel populated from a real scan of root instead
// of static placeholder text.
func NewLiveShell(root string, session *kernel.SessionStore, pluginDirs []string) *Shell {
	s := New(session)

	ws, wsErr := workspaceintel.NewDetector().Scan(root)
	s.RegisterPanel(DomainWorkspaceIntelligence, NewStubPanel("Workspace", workspaceIntelligenceContent(ws, wsErr)))

	s.RegisterPanel(DomainProjectGeneration, NewStubPanel("Generate", projectGenerationContent(pluginDirs)))

	scmModel, scmErr := sourcecontrol.NewProvider(root).Status()
	s.RegisterPanel(DomainSourceControl, NewStubPanel("Source Control", sourceControlContent(scmModel, scmErr)))

	secModel, secErr := securityCenterScan(root)
	s.RegisterPanel(DomainSecurityCenter, NewStubPanel("Security", securityCenterContent(secModel, secErr)))

	s.RegisterPanel(DomainQuality, NewStubPanel("Quality", "Quality — checks (test/lint/format/build) run on demand, not automatically at startup. Press a future run-checks key to invoke core/domains/quality.Runner."))

	devModel, devErr := devenvironment.NewDoctor(pluginDirs...).Diagnose(ws.DeclaredToolchain)
	s.RegisterPanel(DomainDevEnvironment, NewStubPanel("Dev Environment", devEnvironmentContent(devModel, devErr)))

	s.RegisterPanel(DomainAutomation, NewStubPanel("Automation", "Automation — no tasks registered yet. core/domains/automation.Runner has no auto-discovery (Phase B3 scope); tasks are registered by whatever invokes the shell."))

	// TUI itself publishes no domain model (tui.md: "the only domain
	// that is purely a consumer") — its panel is a static about/settings
	// surface, same as NewPlatformShell's stub.
	s.RegisterPanel(DomainTUI, NewStubPanel("About", "Lumo platform shell — Phase A3 skeleton."))

	return s
}

func workspaceIntelligenceContent(m workspaceintel.Model, err error) string {
	if err != nil {
		return "error scanning workspace: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Languages:   %s\n", joinOrNone(m.Languages))
	fmt.Fprintf(&b, "Frameworks:  %s\n", joinOrNone(m.Frameworks))
	fmt.Fprintf(&b, "Monorepo:    %v\n", m.IsMonorepo)
	fmt.Fprintf(&b, "Config:      %s\n", joinOrNone(m.ConfigFilesPresent))
	if len(m.DeclaredToolchain) == 0 {
		b.WriteString("Toolchain:   none declared\n")
	} else {
		b.WriteString("Toolchain:\n")
		for lang, version := range m.DeclaredToolchain {
			fmt.Fprintf(&b, "  %s: %s\n", lang, version)
		}
	}
	return b.String()
}

func projectGenerationContent(pluginDirs []string) string {
	p := projectgeneration.NewProvider(registry.New(pluginDirs...), plugin.NewHost())
	m, err := p.Model()
	if err != nil {
		return "error listing plugins: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Templates (%d):\n", len(m.AvailableTemplates))
	for _, t := range m.AvailableTemplates {
		fmt.Fprintf(&b, "  %s — %s/%s/%s\n", t.Name, t.ProjectType, t.Language, t.Framework)
	}
	fmt.Fprintf(&b, "Capabilities (%d):\n", len(m.AvailableCapabilities))
	for _, c := range m.AvailableCapabilities {
		fmt.Fprintf(&b, "  %s (%s)\n", c.Name, c.CapabilityID)
	}
	return b.String()
}

func sourceControlContent(m sourcecontrol.Model, err error) string {
	if _, notRepo := err.(*sourcecontrol.NotARepositoryError); notRepo {
		return "not a Git repository"
	}
	if err != nil {
		return "error reading Git status: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Branch:    %s\n", m.Branch)
	fmt.Fprintf(&b, "Dirty:     %v\n", m.Dirty)
	fmt.Fprintf(&b, "Ahead/Behind: %d/%d\n", m.Ahead, m.Behind)
	fmt.Fprintf(&b, "Remotes:   %s\n", joinOrNone(m.Remotes))
	fmt.Fprintf(&b, "Worktrees: %d\n", m.WorktreeCount)
	return b.String()
}

func securityCenterScan(root string) (securitycenter.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), vulnScanTimeout)
	defer cancel()
	return securitycenter.NewScanner(root).Scan(ctx, nil)
}

func securityCenterContent(m securitycenter.Model, err error) string {
	if err != nil {
		return "error scanning: " + err.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Gate:            %s\n", m.GateState)
	fmt.Fprintf(&b, "Vulnerabilities: %s (vuln check is timeout-bounded; run manually for a full scan)\n", joinOrNone(m.Vulnerabilities))
	if len(m.SecretsFound) == 0 {
		b.WriteString("Secrets:         none found\n")
	} else {
		fmt.Fprintf(&b, "Secrets:         %d finding(s)\n", len(m.SecretsFound))
		for _, f := range m.SecretsFound {
			fmt.Fprintf(&b, "  %s (%s)\n", f.Path, f.Pattern)
		}
	}
	return b.String()
}

func devEnvironmentContent(m devenvironment.Model, err error) string {
	if err != nil {
		return "error diagnosing environment: " + err.Error()
	}
	var b strings.Builder
	if len(m.InstalledToolVersions) == 0 {
		b.WriteString("Installed: none of the declared toolchains were found on PATH\n")
	} else {
		b.WriteString("Installed:\n")
		for lang, version := range m.InstalledToolVersions {
			fmt.Fprintf(&b, "  %s: %s\n", lang, version)
		}
	}
	fmt.Fprintf(&b, "Missing:   %s\n", joinOrNone(m.MissingTools))
	if len(m.DiagnosticIssues) == 0 {
		b.WriteString("Issues:    none\n")
	} else {
		fmt.Fprintf(&b, "Issues:    %d (remediation available for %d)\n", len(m.DiagnosticIssues), len(m.RemediationsAvailable))
	}
	return b.String()
}

// joinOrNone formats a string slice for a panel line, since an empty
// slice reading as a blank field is more confusing than saying "none".
func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
