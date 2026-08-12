package shell

import "github.com/intruder0007/Lumo/core/kernel"

// The eight domain IDs from docs/architecture/v2-platform-architecture.md
// §1. Defined here, not in core/kernel, because knowing the full domain
// list is a TUI-shell concern (tui.md: "the only domain that is purely a
// consumer") — the kernel itself stays domain-agnostic.
const (
	DomainWorkspaceIntelligence kernel.DomainID = "workspace-intelligence"
	DomainProjectGeneration     kernel.DomainID = "project-generation"
	DomainSourceControl         kernel.DomainID = "source-control"
	DomainSecurityCenter        kernel.DomainID = "security-center"
	DomainQuality               kernel.DomainID = "quality"
	DomainDevEnvironment        kernel.DomainID = "dev-environment"
	DomainAutomation            kernel.DomainID = "automation"
	DomainTUI                   kernel.DomainID = "tui"
)

// stubDomain pairs a domain ID with its spine title and placeholder
// panel content, so NewPlatformShell has one place to define both.
type stubDomain struct {
	id      kernel.DomainID
	title   string
	content string
}

// platformDomains lists the eight domains in the order the Phase A3
// spine displays them. TUI itself is included (tui.md's own panel is
// this shell's "about/settings" surface), matching all eight domains
// in docs/architecture/v2-platform-architecture.md §1.
var platformDomains = []stubDomain{
	{DomainWorkspaceIntelligence, "Workspace", "Workspace Intelligence — not yet implemented (Phase B1)."},
	{DomainProjectGeneration, "Generate", "Project Generation — not yet wrapped in a panel (Phase B4)."},
	{DomainSourceControl, "Source Control", "Source Control — not yet implemented (Phase B1)."},
	{DomainSecurityCenter, "Security", "Security Center — not yet implemented (Phase B2)."},
	{DomainQuality, "Quality", "Quality — not yet implemented (Phase B2)."},
	{DomainDevEnvironment, "Dev Environment", "Dev Environment — not yet implemented (Phase B3)."},
	{DomainAutomation, "Automation", "Automation — not yet implemented (Phase B3)."},
	{DomainTUI, "About", "Lumo platform shell — Phase A3 skeleton."},
}

// NewPlatformShell returns a Shell with all eight platform domains
// registered as stub panels, in spine order. This is the concrete
// Phase A3 exit-criterion artifact: "Shell navigates between 8 stub
// panels."
func NewPlatformShell(session *kernel.SessionStore) *Shell {
	s := New(session)
	for _, d := range platformDomains {
		s.RegisterPanel(d.id, NewStubPanel(d.title, d.content))
	}
	return s
}
