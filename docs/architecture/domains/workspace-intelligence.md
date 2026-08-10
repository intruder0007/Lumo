# Domain: Workspace Intelligence

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B1 ships against it once.

## Ownership

Detecting what a repo/project *is*: languages present, frameworks in use,
monorepo/workspace layout, toolchain versions declared by the project
(not what's installed on the machine — that's Dev Environment), which
config files exist, and structural facts like module boundaries. This is
the base layer every other domain reads instead of re-detecting the same
things — the direct fix for the pattern currently building up inside
`lumo status`.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type WorkspaceModel struct {
    RootPath           string
    Languages          []string          // e.g. ["go", "javascript"]
    Frameworks         []string
    IsMonorepo         bool
    DeclaredToolchain  map[string]string // language -> version the project declares (go.mod, package.json engines, etc.)
    ConfigFilesPresent []string          // relative paths: go.mod, package.json, .git, ...
    LastScanned        time.Time
}
```

## Reads from the Fact Store

Nothing — this is the base layer. No domain sits below it.

## Events emitted

- `model-changed` — whenever a rescan detects a difference from the last
  published model.

## Interface sketch

```go
// Illustrative — not the final API.
type Detector interface {
    Scan(root string) (WorkspaceModel, error)
}
```

## Today → Gap

| Today | Gap |
|---|---|
| Detection logic lives inline inside the `lumo status` command. | No shared model; every consumer (Security Center's vuln row, the SonarQube row, the Git row) re-detects or hardcodes what it needs. |

Phase B1 moves the existing detection logic behind `Detector`, publishing
`WorkspaceModel` — this is a relocation, not a rewrite from scratch
(ADR-0017 Consequences).

## Explicit non-goals

- Does not judge whether a detected toolchain version is adequate,
  outdated, or missing from the machine — that evaluation is Dev
  Environment's job, reading this model as input.
- Does not look at VCS state (branch, dirty, remotes) — that's Source
  Control, even though `.git` presence is itself a fact this domain
  reports (`ConfigFilesPresent`).
- Does not scan for vulnerabilities or secrets — Security Center reads
  `WorkspaceModel` (e.g. which manifest files exist) to know *where* to
  look, but the scanning itself is Security Center's.
