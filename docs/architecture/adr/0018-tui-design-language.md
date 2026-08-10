# ADR-0018: TUI design language — original, not derivative

## Status

Accepted

## Context

ADR-0017 makes the TUI one of eight platform domains: the shell that
unifies Workspace Intelligence, Project Generation, Source Control,
Security Center, Quality, Dev Environment, and Automation into a single
persistent, keyboard-driven session. The existing wizard
(`cli/internal/prompt`: `wizard.go`, `components.go`, `progress.go`, the
theme registry) was built for one linear flow and cannot be grown into a
multi-panel shell by extension alone — Phase A3 builds a new navigation
shell from scratch.

The terminal-UI space this shell competes in already has strong, familiar
conventions (lazygit, k9s, gh-dash, btop, lazydocker). Left unconstrained,
the natural failure mode is convergence: a new panel gets designed by
analogy to a tool everyone already knows, and "original" quietly becomes
"assembled from familiar parts." This ADR makes originality a design
constraint with an enforcement mechanism, not just an aspiration.

## Decision

**The TUI's navigation, layout, and interaction model must be derived from
Lumo's own domain model (ADR-0017 §1), not from any existing tool's
conventions — studied, never imitated.**

Process, required before Phase A3 is considered complete:

1. **Interaction survey.** Document 4-6 reference TUIs (lazygit, k9s,
   gh-dash, btop, lazydocker, and any others reviewed) in terms of the
   *problem* each interaction pattern solves — not a screenshot gallery.
   Example framing: "lazygit's staging-panel keybindings solve
   partial-file-diff selection under a modal-free model" — the problem,
   stated independent of lazygit's specific keys.
2. **Problem extraction, not pattern reuse.** From the survey, list the
   underlying problems Lumo's shell actually has — e.g. "surface eight
   domains' worth of state without a tab bar wide enough to lose
   context," "show a security gate's blocking state without stealing
   focus from whatever panel the user is in." These problems come from
   Lumo's domain model, not from the survey.
3. **Design from Lumo's shape.** Navigation, layout, and keybinding
   grammar are designed to answer Lumo's own problem list. If a design
   choice happens to resemble an existing tool's, that's coincidence to be
   noted, not a target to aim for.
4. **Exit gate.** The design rationale document must be explainable
   without naming any reference tool. "It's like lazygit but for X" is a
   failed design and blocks the Phase A3 exit criterion.

This gate is re-applied at Phase C2 (TUI design language maturity) before
any polish pass ships, so later work can't quietly re-converge on a
reference tool that the initial design successfully avoided.

## Consequences

- Phase A3 cannot close until both the interaction survey and the design
  rationale doc exist and the rationale doc passes the naming-free
  explainability test — this is an explicit, checkable gate, not a
  subjective sign-off.
- The existing wizard's accessibility guarantees (NO_COLOR support,
  minimal/screen-reader-friendly theme, no state encoded in color alone)
  carry forward as constraints on the new shell — originality applies to
  navigation/layout/interaction, not to safety properties that are
  already correct.
- If Phase A3 or C2 produces a design that fails the exit gate, the fix is
  to redo the problem-extraction step (2), not to reskin the derivative
  design — reskinning a copied structure is not originality.
- This ADR does not prescribe a specific layout or keybinding scheme —
  that is Phase A3's/C2's deliverable, not this decision's.
