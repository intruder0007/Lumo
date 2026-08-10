# Domain: Automation

> Status: Experimental (ADR-0017 A1). Interface sketch for Phase A1; the
> real shape is fixed once Phase B3 ships one concrete workflow against
> it (see risk note below — this domain has the highest design risk in
> Phase B).

## Ownership

Task running, hook orchestration, local CI emulation, and background/
scheduled jobs — the platform's answer to "run and watch things." Has no
existing V1 precedent; only the plugin subprocess model underneath
Project Generation is structurally related (same idea: run something,
observe its output), not the same thing.

## Model published to the Fact Store

```go
// Illustrative — not the final API.
type AutomationModel struct {
    AvailableTasks []TaskDef
    RunningJobs    []JobStatus
    RecentRuns     []JobResult
}
```

## Reads from the Fact Store

- `WorkspaceModel` (Workspace Intelligence) and `GenerationModel`
  (Project Generation) — to discover task definitions (e.g. Makefile
  targets, plugin-declared tasks).
- May *invoke* other domains' operations as task bodies (e.g. running
  Quality's `Runner.RunTests` as a task) without owning their result
  models — Automation reports that a task ran and its exit status;
  the domain whose operation it invoked owns the detailed result.

## Events emitted

- `job-started`
- `job-completed`

## Interface sketch

```go
// Illustrative — not the final API.
type Runner interface {
    ListTasks() ([]TaskDef, error)
    RunTask(id string) (JobHandle, error)
}
```

## Today → Gap

| Today | Gap |
|---|---|
| No concept exists. Only the plugin subprocess model (Project Generation) is structurally adjacent. | Whole domain to design. |

Per the Phase B3 plan, scope the first slice to one concrete workflow —
"run the project's test command from the TUI" — before generalizing to
arbitrary tasks, hooks, or scheduling. Generalizing this domain from zero
real usage is the specific over-design risk called out for Phase B3.

## Explicit non-goals

- Does not define what "quality" or "security" checks mean — it can run
  them as tasks (invoking Quality's or Security Center's own interfaces),
  but does not own or duplicate their result models.
- Does not replace the plugin protocol (ADR-0002) — task execution here
  is a distinct concept from template/capability plugins, even if Phase
  C3 later decides to generalize the plugin host to cover both.
