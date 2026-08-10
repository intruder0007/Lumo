# TUI interaction survey and design direction

> Fulfills ADR-0018 steps 1-3 (survey, problem extraction, design-from-
> Lumo's-shape) for Phase A3. This is a design-direction document, not
> a built shell — no code accompanies this file. Building the actual
> shell (Phase A3's remaining, implementation half) is separate work and
> starts only once this direction is reviewed, per the "no coding yet"
> boundary this whole plan operates under.

## 1. Interaction survey

Five reference TUIs, read for the *problem* each pattern solves — not
copied for their specific keys or colors.

| Tool | Problem it solves |
|---|---|
| lazygit | Perform multi-step VCS operations (stage a hunk, amend, rebase) with minimal modes and immediate visual feedback of the resulting state change. |
| k9s | Navigate a large, dynamically changing hierarchy of resource types without losing place, using a persistent command/filter input and fast resource-type switching. |
| gh-dash | Surface several independent, asynchronously-updating lists (PRs, issues, review requests) in one glance without deep navigation, plus quick keyboard actions on list items. |
| btop | Show many concurrently-changing numeric/graph data streams at once without tabbing between them — density over navigation. |
| lazydocker | Switch between resource-type lists (containers, images, volumes) and a detail/log pane for whatever's selected, similar in shape to lazygit but multi-resource. |

## 2. Lumo's own problems (from the domain model, ADR-0017 §1)

Extracted from what Lumo's eight domains actually need to show — not from
the survey above:

- **P1 — Eight domains, no tab-bar-wide-enough-to-lose-context.** Same
  problem class as k9s's hierarchy navigation and lazydocker's resource
  switching, but Lumo's "resources" are whole domains, not items within
  one resource type.
- **P2 — Static facts vs. live state look identical if undesigned.**
  Workspace Intelligence's model changes rarely (a rescan); Source
  Control's and Automation's change while you watch. An interaction model
  that treats both the same either spams "updated" noise on static data
  or under-signals real live state.
- **P3 — A blocking gate (Security Center) must be visible without
  stealing focus.** A vuln scan finishing while the user is heads-down in
  Project Generation must not yank their attention, but must not be
  missable either.
- **P4 — Inspect vs. act needs one consistent grammar, not eight.**
  Some domains are read-then-act (Source Control commit, Dev Environment
  remediate); others are read-only (Workspace Intelligence, Quality
  display today). Each domain inventing its own confirm/act convention
  would fragment the shell's muscle memory.
- **P5 — Background job completions need a home that isn't a dialog.**
  Automation's jobs can finish while the user is on another panel; a
  stacking-dialog model doesn't scale past one concurrent job.
- **P6 — Project Generation is a linear wizard, not a panel like the
  other seven.** Forcing it into the same persistent-panel shape as
  Source Control or Quality would distort a flow that works today.

## 3. Design direction (derived from P1-P6, not from the survey)

- **A persistent spine, not a tab bar.** All eight domains are listed by
  name with a compact status glyph in a strip that stays visible even
  while a panel is focused — solves P1 by keeping "where are the other
  seven" answerable without switching away from what you're doing.
- **Two visual registers for state.** Panels backed by rarely-changing
  models (Workspace Intelligence) show a "last scanned" mark and a manual
  rescan action; panels backed by continuously-changing models (Source
  Control, Automation) show a lightweight live-indicator only while a
  value is actually in motion. Solves P2 by making dynamism itself
  legible instead of uniform.
- **Gate state lives on the spine, not in a popup.** Security Center's
  `GateState` renders as a persistent marker next to its spine entry;
  focus never moves there automatically. Solves P3 — visible, never
  intrusive.
- **One global act-grammar across all eight panels.** A small, consistent
  key convention distinguishes "navigate/select" from "act" (e.g.
  commit, remediate, run task), applied identically regardless of domain,
  so the muscle memory built in Source Control transfers to Dev
  Environment. Solves P4.
- **A recent-activity strip, append-only, dismissible.** Job completions
  (Automation) and other async events post here instead of interrupting
  whatever panel has focus. Solves P5.
- **Project Generation opens as a focused, full-screen mode from its
  spine entry**, temporarily hiding the spine for the duration of the
  wizard, returning to the persistent shell on completion — the one
  domain allowed a different interaction shape, because its underlying
  flow (ADR-0002-driven generation) genuinely is linear. Solves P6.

## 4. ADR-0018 exit-gate self-check

The direction above is stated without naming any reference tool, and each
element traces to a Lumo-specific problem (P1-P6), not a survey entry.
Coincidental resemblance to any one tool's specific mechanism (e.g. a
persistent side list) is noted as coincidence: the survey's role was
extracting *problems*, and multiple tools in §1 solve adjacent problems by
convergent evolution — that's expected, not evidence of imitation, as
long as Lumo's version follows §2's reasoning rather than any one tool's
implementation.

## 5. What's still open

This document proposes a direction; it does not fix pixel-level layout,
exact key bindings, or color/theme rules (those extend the existing
`NO_COLOR`/minimal-theme accessibility guarantees, per ADR-0018
Consequences, and are Phase A3 implementation decisions once building
starts). Phase A3's shell skeleton, and Phase A2's kernel skeleton, are
the next deliverables — both are code, not architecture, and start only
on explicit go-ahead per this plan's "no coding yet" scope.
