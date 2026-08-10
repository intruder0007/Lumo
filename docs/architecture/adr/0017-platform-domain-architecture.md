# ADR-0017: Platform domain architecture and kernel

## Status

Accepted

## Context

Lumo V1 (through 0.4.0 / `lumo-cli@1.0.0`) is a scaffolding CLI with a
wizard: `lumo new` runs a plugin-driven flow and exits. Work since then
(`lumo status`, dependency-vulnerability row, Git/GitHub/SonarQube checks)
has been adding a second identity — "tell me about my project" — as ad hoc
branches inside one status command, each new signal its own `if` block with
no shared model.

`docs/architecture/v2-platform-architecture.md` sets the target: Lumo
repositions as a terminal-first dev platform with eight bounded domains
(Workspace Intelligence, Project Generation, Source Control, Security
Center, Quality, Dev Environment, Automation, TUI) sitting on a shared
platform kernel. This ADR ratifies that domain/kernel split and resolves
the three open product questions the plan raised, so Phase A execution can
start without an unstated assumption.

## Decision

**Adopt the eight-domain model with a platform kernel as the seam between
domains, per `v2-platform-architecture.md` §1-2.** Each domain is a Go
package with an explicit public interface; no domain reaches into another
domain's internals; cross-domain communication goes through the kernel's
Workspace Model and event bus. This mirrors the existing `cli`→`core`→
`sdk/go` boundary discipline already enforced by `go.mod` (ADR-0002-style
module boundaries), applied one level deeper.

Three sequencing decisions, resolving the plan's open questions:

1. **Plugin host generalization: deferred to Phase C3, not built in
   Phase B.** Security Center, Quality, Dev Environment, and Automation
   ship as in-process domain implementations in Phase B. The plugin
   protocol (JSON-RPC/stdio, ADR-0002) stays scoped to Project Generation
   until Phase C3 evaluates whether a concrete third-party need justifies
   opening it to other domains. Rationale: this repo's existing pattern is
   additive-when-needed, not speculative (remote registry, non-Go SDKs,
   and capability visibility are all deferred the same way per
   `roadmap.md`) — building a generalized plugin surface for domains with
   no third-party plugin author yet would repeat that mistake.

2. **Security Center's B2 work does not block on cosign/Sigstore signing.**
   `SECURITY.md`'s plugin-signing work (targeted v0.8.0 on the V1 track) is
   independent of Security Center's other functions (dependency scanning,
   secret detection). B2 ships Security Center with a stub trust field
   (`signature: unverified`) and a policy-gate structure that has a slot
   for signature state, not a working signer. When the v0.8.0-track signing
   work lands, it plugs into that slot — no Security Center redesign
   needed, per the interface stability goal in A1.

3. **Versioning: additive during Phase B, major cut at Phase C4.** The TUI
   ships as an additive `lumo tui` command alongside the existing
   scriptable surface throughout Phase B — `lumo new`, `lumo doctor`,
   `lumo status` keep their current Stable contracts unchanged (ADR-0013).
   The breaking identity change ("Lumo is a platform, not a CLI wizard")
   is declared once, at the Phase C4 hardening/release exit criterion, as
   the next major version. This avoids forcing a version bump before the
   platform experience is actually complete enough to justify one.

## Consequences

- Phase A1 deliverable: one interface spec per domain under
  `docs/architecture/domains/`, marked Experimental until a Phase B
  sub-phase ships against it at least once (interfaces are expected to
  move before B-phase implementation, not frozen at A1).
- Phase A2 (kernel skeleton — Workspace Model, event bus, config/session
  store) is scoped to *decoupled infrastructure with zero domain
  consumers* as its own exit criterion, specifically to catch
  over-designed kernel APIs before any domain depends on them.
- `lumo status`'s existing detection logic is the first thing migrated
  into the Workspace Model (Phase B1) — this ADR does not ask for a
  rewrite from scratch, it asks for relocation behind a shared interface.
- Security Center ships in Phase B2 with a documented trust-model gap
  (no real signature verification) — this is a known, accepted limitation
  until the v0.8.0-track signing work lands, not a silent omission.
- No existing Stable surface (ADR-0013) changes as a result of this ADR.
  A future ADR is required before any Stable command/flag contract
  changes as part of platform work.
- This ADR does not decide Phase C3's extensibility question — it only
  fixes *when* that decision gets made, not what it will conclude.
