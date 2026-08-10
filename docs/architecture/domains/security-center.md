# Domain: Security Center

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B2 ships against it once.

## Ownership

Dependency vulnerability scanning, secret detection, plugin trust/signing
status, and supply-chain posture — consolidated into one policy-gate
concept, replacing the single ad hoc vuln row.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type SecurityModel struct {
    Vulnerabilities []VulnFinding
    SecretsFound    []SecretFinding
    PluginTrust     map[string]TrustState // keyed by plugin name; see below
    GateState       string                // "pass" | "warn" | "block"
}

type TrustState struct {
    Signed   bool   // false until v0.8.0-track signing work lands (ADR-0017 Decision 2)
    Verified bool
}
```

## Reads from the Fact Store

- `WorkspaceModel` (Workspace Intelligence) — which manifest files exist,
  to know where to scan.
- `GenerationModel` (Project Generation) — which plugins are installed,
  to populate `PluginTrust`.

## Events emitted

- `gate-state-changed` — `GateState` transitions (e.g. pass → block on a
  newly found vulnerability).

## Interface sketch

```go
// Illustrative — not the final API.
type Scanner interface {
    Scan(ws WorkspaceModel) (SecurityModel, error)
}
```

## Today → Gap

| Today | Gap |
|---|---|
| A single vuln row, gated behind `--offline`. | No secret scanning, no plugin trust surface, no policy-gate concept. |

Per ADR-0017 Decision 2, `PluginTrust.Signed`/`Verified` ship as an
honest stub (`false`/`false`) in Phase B2 — Security Center does not block
on the separate v0.8.0-track cosign/Sigstore signing work landing first.
When that work lands, it populates this same field; no Security Center
redesign is needed.

## Explicit non-goals

- Does not implement the signer itself — that's the V1-track SECURITY.md
  work; this domain only has a slot for its result.
- Does not run tests, lint, or coverage — that's Quality, even though
  both concepts feed into a "should this commit/release proceed"
  question. Each domain reports its own gate state; unifying them into
  one release decision is Automation/Phase C1 work, not this domain
  merging with Quality.
- Does not decide the plugin protocol's future scope (Phase C3) —
  Security Center is a candidate *consumer* of a generalized plugin host
  if that decision is made, not the decision-maker.
