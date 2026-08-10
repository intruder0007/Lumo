# Domain: Project Generation

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B4 ships against it once.

## Ownership

Scaffolding new projects and adding capabilities to existing ones, via the
existing plugin protocol (JSON-RPC/stdio, ADR-0002). This domain **is**
Lumo V1 — the `core` engine, plugin-host, registry, `sdk/go`, and
`templates/`. Becoming one domain among eight does not change its
internals; it changes what else exists alongside it.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type GenerationModel struct {
    AvailableTemplates    []TemplateInfo
    AvailableCapabilities []CapabilityInfo
    LastGeneration        *GenerationRecord // nil if none this session
}
```

## Reads from the Fact Store

- `WorkspaceModel` (Workspace Intelligence) — when adding a capability to
  an *existing* project (as opposed to `lumo new`'s greenfield case), to
  know what's already there.

## Events emitted

- `generation-completed` — a template or capability finished applying.

## Interface sketch

This is intentionally close to what already exists in `core` — the point
of B4 is wrapping, not redesigning:

```go
// Illustrative — matches the shape of the existing core engine.
type Engine interface {
    ListTemplates() []TemplateInfo
    ListCapabilities() []CapabilityInfo
    Generate(req GenerateRequest) (GenerateResponse, error)
    ApplyCapability(req ApplyRequest) (ApplyResponse, error)
}
```

## Today → Gap

| Today | Gap |
|---|---|
| Full V1 implementation: `core` engine, plugin-host, registry, `sdk/go`, `templates/`, `plugins/builtin/`. `lumo new` is Stable (ADR-0013). | None structural. The gap is presentational: no TUI panel wraps it yet — it's reachable only via the linear wizard. |

Phase B4 wraps the existing engine in a TUI panel without touching the
Stable `lumo new` non-interactive path (ADR-0017 Decision 3; this is the
domain with the most existing Stable-surface exposure, per the plan's B4
risk note).

## Explicit non-goals

- Does not scan an existing repo for facts beyond what Workspace
  Intelligence's `WorkspaceModel` already provides — no duplicate
  detection.
- Does not evaluate plugin trust/signing — Security Center owns that,
  reading which plugins are installed from this domain's model.
- Does not decide the plugin protocol's future scope (whether other
  domains get plugin support) — that's the Phase C3 decision recorded in
  ADR-0017, owned collectively, not by this domain unilaterally.
