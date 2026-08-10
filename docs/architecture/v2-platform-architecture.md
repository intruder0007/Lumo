# Lumo v2: Terminal-First Dev Platform — Architecture Plan

> **Scope of this document: architecture and roadmap only. No implementation.**
> Nothing here authorizes code changes. It exists to fix the target shape of
> the system before further features are built against it, and to be
> ratified (or amended) via ADRs before Phase A execution begins.

## 0. Mandate

Lumo V1 (through 0.4.0 / the `lumo-cli@1.0.0` wrapper) is a **scaffolding
CLI with a wizard**: `lumo new` runs a plugin-driven interactive flow and
exits. Recent work (`lumo status`, the dependency-vulnerability row, Git/
GitHub/SonarQube checks) is organically growing a second identity —
"tell me about my project" — as ad hoc additions bolted onto a single
status command. That pattern doesn't scale: each new signal becomes another
`if` branch in one file instead of a bounded subsystem with its own model
and tests.

This plan repositions Lumo as a **terminal-first dev platform**: one
persistent, keyboard-driven TUI session that houses eight clearly bounded
domains, sitting on a shared platform kernel, with the existing scriptable
`lumo <command>` surface preserved for CI/non-interactive use. Feature work
pauses; architecture ratification and skeleton-building (Phase A) come
first.

## 1. Domain Model

Eight domains, each a Go package with an explicit public interface
(mirroring the existing `cli`→`core`→`sdk/go` boundary discipline enforced
today by `go.mod`, not just convention — see `docs/architecture/overview.md`).
No domain reaches into another's internals; cross-domain communication goes
through the platform kernel (§2).

| Domain | Owns | Today | Gap |
|---|---|---|---|
| **Workspace Intelligence** | Detecting what a repo/project *is*: languages, frameworks, monorepo layout, toolchain versions, config files present, health signals. Produces the shared "Workspace Model" every other domain reads. | Ad hoc detection logic embedded directly in the `lumo status` command. | No shared model; each consumer re-detects facts independently. |
| **Project Generation** | Scaffolding new projects and adding capabilities to existing ones via the plugin protocol (JSON-RPC/stdio, ADR-0002). | This *is* V1 — `core` engine, plugin-host, registry, `sdk/go`, templates. | Currently the product's whole identity; must become one domain among eight without breaking the Stable `lumo new` surface. |
| **Source Control** | Git and GitHub state and actions: status, diff, branch, commit, PR/review context. | A read-only "Git row" in `lumo status`. | No write actions, no GitHub-side model beyond a status check. |
| **Security Center** | Dependency vulnerability scanning, secret detection, plugin trust/signing (cosign/Sigstore — flagged as not-yet-done in `SECURITY.md`, targeted v0.8.0), supply-chain posture. | A single vuln row, gated behind `--offline`. | No policy gate concept, no secret scanning, no signing verification. |
| **Quality** | Test execution, coverage, lint, format, build health; the substance behind ADR-0006's release process. | SonarQube row in `lumo status` only. | No interactive drill-down, no local gate before commit/release. |
| **Dev Environment** | Toolchain/version management, dependency install/audit, env/secrets config, `lumo doctor` diagnostics. | `lumo doctor`, `--verbose` diagnostics. | Diagnostic-only; no remediation actions. |
| **Automation** | Task running, hook orchestration, local CI emulation, background/scheduled jobs. | Doesn't exist as a concept; only the plugin subprocess model exists underneath Project Generation. | Whole domain to design. |
| **TUI** | The single presentation shell unifying all domains into navigable panels. | The `lumo new` wizard (`cli/internal/prompt`: `wizard.go`, `components.go`, `progress.go`, theme registry). | Built for one linear flow, not a persistent multi-panel session. Must be visually and interactionally **original** (§4). |

Non-goal of this redesign: it does not reopen V1's own deferred list
(`docs/architecture/roadmap.md`) — remote plugin registries, non-Go
transports, telemetry, auto-update, i18n stay deferred. Some entries on
that list (public theme API, capability-to-capability visibility) *will*
need revisiting once Automation and cross-domain events exist — flagged as
Phase C decisions, not decided here.

## 2. Layered Architecture

```
                    +----------------------------------------+
                    |               TUI (shell)               |
                    |  panels: Workspace / Generate / SCM      |
                    |  Security / Quality / Env / Automation   |
                    +-----------------+------------------------+
                                      | reads/dispatches via kernel API
                    +-----------------v------------------------+
                    |             Platform Kernel               |
                    |  * Workspace Model (shared fact store)    |
                    |  * Event bus (fact/state change -> notify)|
                    |  * Plugin host (generalized beyond templates) |
                    |  * Config + session store                 |
                    +--+--------+--------+--------+--------+---+
                       |        |        |        |        |
                 +-----v-+ +----v--+ +---v--+ +---v---+ +--v------+
                 |Workspace| |Project| |Source| |Security| |Quality /|
                 |Intel.   | |Gen.   | |Control| |Center | |Dev Env /|
                 |         | |(core) | |      | |       | |Automation|
                 +---------+ +-------+ +------+ +-------+ +---------+
```

Key decisions this implies:

- **The Workspace Model is the only source of truth for "what is this
  project."** Security Center doesn't re-parse `go.mod`; it asks the
  kernel. This directly retires the duplicated-detection pattern building
  up in `lumo status`.
- **The plugin host generalizes.** Today it only runs template/capability
  plugins for Project Generation. Long-run, Security/Quality/Automation
  plugins are the same subprocess+JSON-RPC contract with a different
  capability namespace — not a second protocol. Whether that happens in
  Phase B or C is an open question (§7), not decided here.
- **The scriptable CLI surface is not replaced, it's re-homed.** `lumo new`,
  `lumo doctor`, `lumo status` keep working non-interactively (CI, scripts,
  the existing Stable-surface guarantee in `docs/architecture/api-compatibility.md`)
  by calling the same kernel + domain APIs the TUI panels call. One
  platform, two presentations.

## 3. Cross-Cutting Constraints (carried forward from V1, non-negotiable)

- **Offline-first.** No domain requires network access for its core
  function; Security Center's vulnerability data source must degrade
  explicitly (already the pattern behind today's `--offline` gate), not
  fail silently.
- **Accessible.** `NO_COLOR`, minimal/screen-reader-friendly theme, no
  state encoded in color alone — applies to every new TUI panel, not just
  the wizard.
- **Stable-surface discipline.** Any command/flag surface currently Stable
  per `api-compatibility.md` keeps its contract; new domains launch
  Experimental and graduate deliberately (mirrors ADR-0013).
- **Trusted-but-unsandboxed plugin model stays as-is** unless Security
  Center's Phase B work produces a concrete ADR to change it (signing is
  additive verification, not a sandboxing redesign).

## 4. TUI Originality Mandate

The TUI is the one domain where imitation is an explicit, named risk. This
category has strong existing reference points (lazygit, k9s, gh-dash, btop,
lazydocker) — study them, do not converge on them.

**Process requirement for Phase A3 / C2:**

1. Produce a written interaction survey of 4-6 reference TUIs: what
   problem each interaction pattern solves, not just what it looks like.
2. From the survey, extract *problems* (e.g. "how do you show five domains
   worth of state without a 10-tab bar") — not solutions to copy.
3. Design Lumo's navigation, layout, and keybinding model from Lumo's own
   domain model (§1) and workflows, so the shape follows from what Lumo
   actually needs to show — eight domains behind one Workspace Model — not
   from a genre convention.
4. Exit gate: the design must be explainable without reference to any
   specific existing tool. "It's like lazygit but for X" is a failed
   design.

## 5. Phase Roadmap

Three phases, each with sub-phases. Every sub-phase has goals, deliverables,
risks, and an exit criterion that gates moving on — no sub-phase starts
before the prior one's exit criterion is met.

### Phase A — Foundation: Ratify Boundaries, Build the Kernel Skeleton

**Intent:** stop ad hoc accretion; get the domain contracts and kernel
skeleton in place before any domain logic moves.

#### A1 — Domain Contracts
- **Goals:** Write the interface for each of the 8 domains (inputs,
  outputs, ownership boundary) as an ADR set, without implementing them.
- **Deliverables:** `ADR-0017` (domain architecture + kernel), one
  interface spec per domain under `docs/architecture/domains/`.
- **Risks:** Interfaces drawn too early calcify wrong before real usage
  patterns exist. Mitigate by marking every domain interface Experimental
  until Phase B ships against it once.
- **Exit criterion:** All 8 interfaces reviewed and merged as ADRs; no
  domain's interface depends on another domain's internals.

#### A2 — Platform Kernel Skeleton
- **Goals:** Stand up the Workspace Model store, event bus, and
  config/session store as empty-but-real infrastructure — no domain logic
  inside them yet.
- **Deliverables:** Kernel package with the three subsystems above, wired
  to nothing but a smoke test.
- **Risks:** Building infrastructure with no consumer risks over-design.
  Mitigate by kernel APIs only growing methods A3/B-phase domains actually
  call, not speculative ones.
- **Exit criterion:** Kernel compiles standalone, has its own test suite,
  zero domain code depends on it yet (proves it's decoupled, not proves
  it's useful).

#### A3 — TUI Shell Skeleton + Originality Survey
- **Goals:** Complete the originality survey (§4); build the navigation
  shell (panel switching, layout, keybinding model) with all 8 panels
  stubbed as placeholders.
- **Deliverables:** Interaction survey doc, shell skeleton, `ADR-0018`
  (TUI design language).
- **Risks:** Team defaults to a known pattern under time pressure. Mitigate
  via the explicit exit gate in §4.
- **Exit criterion:** Shell navigates between 8 stub panels; design
  rationale doc passes the "explainable without naming another tool" test.

**Phase A exit criterion (overall):** ADR-0017/0018 ratified, kernel
skeleton exists and is tested, TUI shell navigates real (stubbed) panels.
No domain has real logic yet — that's Phase B.

---

### Phase B — Domain Build-Out

**Intent:** implement each domain against the kernel, in an order that
lets each land independently shippable, building on what already
partially exists rather than discarding it.

#### B1 — Workspace Intelligence + Source Control
- **Goals:** Move `lumo status`'s detection logic into the Workspace
  Model; build Source Control from a read-only Git row into a full
  domain (status, diff, branch, commit actions; GitHub PR/review context).
- **Deliverables:** Workspace Model populated from real detection; Source
  Control panel with read + basic write actions.
- **Risks:** Git write actions are inherently higher-blast-radius than
  read-only status; needs its own confirm/undo model before shipping.
- **Exit criterion:** Every other domain's future data comes from the
  Workspace Model, not its own detection code — verified by code review,
  not just intent.

#### B2 — Security Center + Quality
- **Goals:** Generalize the vuln row into a full Security Center (secret
  detection, plugin trust/signing status surfaced, policy gate concept);
  build Quality from the SonarQube row into test/coverage/lint drill-down.
- **Deliverables:** Security Center panel with gate states (pass/warn/
  block); Quality panel with per-check drill-down.
- **Risks:** Signing/trust verification depends on the cosign/Sigstore work
  flagged not-yet-done in `SECURITY.md` (targeted v0.8.0 in the V1 track) —
  this sub-phase may be blocked on that landing first; sequencing needs a
  decision at Phase B kickoff, not assumed here.
- **Exit criterion:** Both panels operate off Workspace Model + kernel
  event bus (react to file changes), not polling.

#### B3 — Dev Environment + Automation
- **Goals:** Extend `lumo doctor` diagnostics into a Dev Environment
  domain with remediation actions (not just detection); design and build
  Automation domain (task running, hook orchestration, local CI
  emulation) on the generalized plugin host.
- **Deliverables:** Dev Environment panel with fix actions; Automation
  panel running at least one real task type end to end.
- **Risks:** Automation is the domain with no existing V1 precedent —
  highest design risk in Phase B. Mitigate by scoping B3's Automation
  slice to one concrete workflow (e.g. "run project's test command from
  the TUI") before generalizing.
- **Exit criterion:** A user can go from "environment broken" to "fixed"
  and from "run my tests" to "seeing results," inside the TUI, without
  dropping to a shell.

#### B4 — Project Generation Migration
- **Goals:** Port the existing wizard into a TUI panel/mode without
  breaking the Stable `lumo new` scriptable path.
- **Deliverables:** Generation panel wrapping the existing `core` engine;
  `lumo new` non-interactive path untouched and re-verified.
- **Risks:** This is the domain with the most existing Stable-surface
  exposure (ADR-0013) — regression here breaks real users, not just
  internal consumers.
- **Exit criterion:** Existing `lumo new` CI/scripted usage passes
  unchanged; wizard is also reachable as a TUI panel.

**Phase B exit criterion (overall):** all 8 domains have real
implementations behind the kernel; scriptable CLI surface unbroken;
TUI shell's stub panels are now real.

---

### Phase C — Platform Cohesion & Maturity

**Intent:** move from "eight domains that work" to "one coherent platform"
— cross-domain intelligence, TUI polish, extensibility, and a stable cut.

#### C1 — Cross-Domain Intelligence
- **Goals:** Wire the event bus so domains react to each other (e.g. a
  new dependency triggers both Quality and Security Center, not just the
  domain a user happened to be looking at).
- **Deliverables:** At least 2 real cross-domain event flows in
  production, documented as the pattern for future ones.
- **Risks:** Event bus becomes an unbounded everything-notifies-everything
  system. Mitigate with an explicit allow-list of event types per domain,
  reviewed as part of this sub-phase, not left implicit.
- **Exit criterion:** Cross-domain flows are traceable (can answer "why
  did this panel update") without reading kernel source.

#### C2 — TUI Design Language Maturity
- **Goals:** Full interaction polish pass against the Phase A3 design
  language — consistent keybinding grammar, layout system, motion/state
  feedback across all 8 panels.
- **Deliverables:** Updated `ADR-0018` reflecting what actually shipped;
  keybinding reference doc.
- **Risks:** Polish work re-litigating Phase A originality decisions late.
  Mitigate by treating A3's design rationale as the constraint, not a
  suggestion.
- **Exit criterion:** A new panel added after this point can follow a
  written design-language spec without inventing new conventions.

#### C3 — Extensibility
- **Goals:** Decide and, if approved, execute generalizing the plugin
  protocol so Security/Quality/Automation accept third-party plugins the
  way Project Generation does today.
- **Deliverables:** ADR deciding scope (may conclude "not yet" — that's a
  valid outcome); if approved, protocol extension + one reference plugin
  per newly-opened domain.
- **Risks:** This is the sub-phase most likely to reopen V1 non-goals
  (remote registry, capability visibility) — treat as a fresh decision,
  not an assumption this plan makes for you.
- **Exit criterion:** Explicit ADR decision recorded, whichever way it
  goes.

#### C4 — Hardening & Release
- **Goals:** Stabilize the platform surface, update all user-facing docs
  (this repo's known doc-drift problem — README/SECURITY/status docs
  disagreeing — must not repeat for the platform release), cut the next
  major version.
- **Deliverables:** Stable-surface declarations for graduated domain
  APIs; single source-of-truth status doc; release.
- **Risks:** Doc drift recurrence (this repo has a documented history of
  it). Mitigate by making the release checklist include a cross-doc
  consistency pass as a named, non-skippable step.
- **Exit criterion:** One version number, one status story, across
  README/CHANGELOG/SECURITY/all architecture docs.

## 6. Governance

- Phase A1/A3 each produce an ADR (`0017`, `0018`) before any Phase B work
  starts — this repo's existing culture (16 ADRs, all Accepted before
  build) is the model, not a formality to skip.
- Each sub-phase's exit criterion is a gate, not a target date — no
  fixed-calendar commitment is made in this document.
- This document supersedes no existing ADR. Where it touches a V1 non-goal
  (theme API, capability visibility), the relevant sub-phase (C3) must
  produce its own ADR rather than this plan deciding it by omission.

## 7. Open Questions — resolved

Resolved by [ADR-0017](adr/0017-platform-domain-architecture.md):

1. **Plugin host generalization timing** — deferred to Phase C3. Domains
   built in Phase B are in-process, not plugin-hosted, until a concrete
   third-party need justifies opening the protocol.
2. **Signing/trust dependency** — Security Center's B2 work does not
   block on the V1-track cosign/Sigstore work; ships with a stub trust
   field and retrofits when signing lands.
3. **Versioning** — additive `lumo tui` command throughout Phase B;
   breaking major-version cut reserved for the Phase C4 exit criterion.

TUI originality process (§4) is formalized in
[ADR-0018](adr/0018-tui-design-language.md).
