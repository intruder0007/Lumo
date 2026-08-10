# Domain: Dev Environment

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B3 ships against it once.

## Ownership

Toolchain/version management, dependency install/audit, env/secrets
config, and `lumo doctor` diagnostics — answering "is my machine ready to
work on this project," and, new in Phase B3, doing something about it.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type DevEnvModel struct {
    InstalledToolVersions map[string]string // what's actually on the machine
    MissingTools          []string
    DiagnosticIssues      []DoctorIssue
    RemediationsAvailable []Remediation // new in B3 — doctor gains fix actions
}
```

## Reads from the Fact Store

- `WorkspaceModel` (Workspace Intelligence) — `DeclaredToolchain`, to
  compare what the project needs against what's installed.

## Events emitted

- `diagnostic-changed`

## Interface sketch

```go
// Illustrative — not the final API.
type Doctor interface {
    Diagnose(ws WorkspaceModel) (DevEnvModel, error)
    Remediate(issueID string) error // new in B3; Diagnose-only today
}
```

## Today → Gap

| Today | Gap |
|---|---|
| `lumo doctor`, `--verbose` diagnostics. | Diagnostic-only; no remediation actions. |

## Explicit non-goals

- Does not scan for vulnerabilities in dependencies — that's Security
  Center, even though both concepts touch "dependencies." Dev Environment
  cares whether a dependency is *installed and the right version*;
  Security Center cares whether it's *safe*.
- Does not run the project's own test suite — Quality's job, even though
  a missing test tool would surface here as a `MissingTools` entry that
  Quality's `Runner` would fail without.
- Does not manage VCS state or actions.
