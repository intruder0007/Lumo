# Domain: Source Control

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B1 ships against it once.

## Ownership

Git and GitHub state and actions: status, diff, branch, commit, and
PR/review context. Owns *dynamic* VCS state — Workspace Intelligence owns
the static fact that a `.git` directory exists at all
(`ConfigFilesPresent`); Source Control owns what that repo is currently
doing.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type SourceControlModel struct {
    Branch         string
    Dirty          bool
    Ahead, Behind  int
    Remotes        []string
    PullRequest    *PullRequestInfo // nil if none open for this branch
    WorktreeCount  int
}
```

## Reads from the Fact Store

- `WorkspaceModel` (Workspace Intelligence) — root path, to locate the
  repository; does not re-derive whether a `.git` directory exists.

## Events emitted

- `branch-changed`
- `commit-created`

## Interface sketch

```go
// Illustrative — not the final API.
type Provider interface {
    Status() (SourceControlModel, error)
    Diff(ref string) (string, error)
    Commit(message string, files []string) error
    // Additional write actions (branch, push) are a B1 design detail,
    // not fixed here.
}
```

## Today → Gap

| Today | Gap |
|---|---|
| Read-only "Git row" inside `lumo status`. | No write actions; no GitHub-side model beyond a status check. |

Git write actions are inherently higher blast-radius than read-only
status (plan's B1 risk note) — B1 needs its own confirm/undo model before
any write action (`Commit`, future `Push`/`Branch`) ships, consistent with
this repo's "confirm before hard-to-reverse actions" norm.

## Explicit non-goals

- Does not detect *whether* a project is a git repository — that's a
  `WorkspaceModel.ConfigFilesPresent` fact from Workspace Intelligence.
  Source Control assumes a repo exists once invoked.
- Does not run quality checks against a diff (e.g. lint-on-commit) — that
  is Quality's concern; Automation may wire the two together via the
  event bus in Phase C1, but Source Control itself stays scoped to VCS
  state and actions.
- Does not manage plugin trust or signing for anything it touches.
