# Domain: Quality

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B2 ships against it once.

## Ownership

Test execution, coverage, lint, format, and build health — the substance
behind ADR-0006's release process, made interactive instead of a single
external-tool status row.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type QualityModel struct {
    TestResults *TestSummary // nil if not yet run this session
    Coverage    float64      // 0 if unknown
    LintFindings []LintFinding
    FormatClean  bool
    BuildStatus  string // "unknown" | "passing" | "failing"
}
```

## Reads from the Fact Store

- `WorkspaceModel` (Workspace Intelligence) — declared language/toolchain,
  to know which test/lint/build runner applies.

## Events emitted

- `quality-gate-changed` — any of `TestResults`, `BuildStatus`, or
  `LintFindings` crosses a pass/fail boundary.

## Interface sketch

```go
// Illustrative — not the final API.
type Runner interface {
    RunTests() (TestSummary, error)
    RunLint() ([]LintFinding, error)
    CheckFormat() (bool, error)
    Build() (string, error) // returns BuildStatus
}
```

## Today → Gap

| Today | Gap |
|---|---|
| SonarQube row inside `lumo status` only. | No interactive drill-down; no local gate before commit/release. |

## Explicit non-goals

- Does not evaluate security posture (vulnerabilities, secrets) — that's
  Security Center, even though both report into a similar pass/warn/block
  shape. Each domain's gate stays independently owned.
- Does not itself become a general task runner — `Runner`'s methods are
  specific, known check types. Arbitrary user-defined tasks (a custom
  script, a deploy step) are Automation's concern; Automation may invoke
  Quality's `Runner` as one of its task types, not the other way around.
- Does not manage toolchain installation — if a required test runner is
  missing, that's a Dev Environment diagnostic, not a Quality concern to
  fix.
