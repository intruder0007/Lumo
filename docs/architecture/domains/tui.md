# Domain: TUI

> Status: Experimental (ADR-0017 A1 / ADR-0018). Interface sketch for
> Phase A3; the real shape is fixed once Phase A3's shell navigates real
> stub panels and passes the ADR-0018 originality exit gate.

## Ownership

The single presentation shell unifying all seven other domains into
navigable panels: layout, keyboard-first navigation, and per-panel
rendering. The only domain that is purely a consumer — it computes no
facts of its own.

## Model published to the Fact Store

None. TUI does not publish a domain model; it owns ephemeral state
(focused panel, scroll position, last-run command) through the kernel's
**Session Store** (`kernel.md` §3), not the Fact Store — that state is
UI-specific and not something another domain should read as a "fact."

## Reads from the Fact Store

All seven other domains' models: `WorkspaceModel`, `GenerationModel`,
`SourceControlModel`, `SecurityModel`, `QualityModel`, `DevEnvModel`,
`AutomationModel`.

## Events emitted

None domain-level (Session Store writes are local, not broadcast).
Subscribes to every other domain's `model-changed`-family events to
know when to re-render a panel.

## Interface sketch

```go
// Illustrative — not the final API.
type Shell interface {
    RegisterPanel(domain DomainID, panel Panel)
    Run() error
}

type Panel interface {
    Render(model any) string // or a richer draw call — TBD in A3
    HandleKey(key Key) error
}
```

## Today → Gap

| Today | Gap |
|---|---|
| The `lumo new` wizard (`cli/internal/prompt`: `wizard.go`, `components.go`, `progress.go`, theme registry) — built for one linear flow. | No persistent multi-panel session; wizard's flow model doesn't generalize to "seven domains, any of which the user might check at any time." |

Phase A3 builds the navigation shell with all 7 domain panels stubbed as
placeholders (Project Generation is the 8th domain but is itself a panel
the shell hosts, same as the others) — no domain logic behind them yet.

## Explicit non-goals

- Does not compute any domain's model itself — pure presentation over
  kernel-read data. If a panel needs a fact no domain publishes yet,
  that's a gap in the domain's spec, not something the TUI works around
  by reaching into a domain's internals.
- Does not decide its own visual/interaction design here — that's
  governed entirely by ADR-0018's originality process (survey → problem
  extraction → design → naming-free exit gate), not by this ownership
  doc.
- Does not replace the existing scriptable CLI surface — `lumo new`,
  `lumo doctor`, `lumo status` keep working non-interactively
  (ADR-0017 Decision 3); the TUI is an additional presentation, not a
  required one.
