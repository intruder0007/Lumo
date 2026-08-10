# Platform kernel — shared contract

> Status: Experimental (ADR-0017 A1). This is the seam every domain spec
> in this directory is written against. It is a contract sketch for
> Phase A2, not an implementation — signatures here are illustrative and
> expected to move once a real domain (Phase B) ships against them.

The kernel is deliberately small: three subsystems, no domain logic. Its
job is to let eight independently-owned domains stay independently owned
while still seeing each other's state.

## 1. Fact Store

Each domain publishes exactly **one** canonical model — its complete,
current view of its own concern — into the Fact Store. Other domains read
it; only the owning domain writes it. This is what replaces the
`lumo status` pattern of every consumer re-detecting the same facts.

```go
// Illustrative — not the final API.
type FactStore interface {
    Publish(domain DomainID, model any)
    Get(domain DomainID) (model any, ok bool)
}
```

Rules:

- A domain's model is a plain data snapshot (a struct), not a live handle
  into the domain's internals — readers get a copy, never a reference
  they could mutate.
- A domain publishes its whole model on every update, not a diff. Diffing
  is the event bus's job (below), not the store's.
- No domain may publish another domain's model, even temporarily during a
  migration — see the Source Control / Workspace Intelligence boundary
  note in `source-control.md` for why this matters concretely.

## 2. Event Bus

Domains don't poll the Fact Store; they subscribe to change notifications.

```go
// Illustrative — not the final API.
type Event struct {
    Domain DomainID
    Kind   string // e.g. "model-changed", "dependency-added"
}

type EventBus interface {
    Publish(e Event)
    Subscribe(domain DomainID, kinds []string, handler func(Event))
}
```

Rules (ratified as a Phase C1 constraint, applied from A2 onward so it
isn't retrofitted later):

- Every domain declares an explicit allow-list of event kinds it emits.
  There is no wildcard "publish anything" escape hatch — this is the
  mitigation for the "everything notifies everything" risk called out in
  the Phase C1 plan.
- A handler that panics or blocks does not take down the bus or other
  subscribers — the kernel isolates dispatch per subscriber. (Exact
  isolation mechanism is a Phase A2 implementation decision, not fixed
  here.)
- Event payloads carry enough to identify *what* changed
  (`Kind`, `Domain`) but not the full model — a handler that needs the
  new state calls `FactStore.Get`, so the store stays the single source
  of truth for "current state" and the bus stays a notification channel.

## 3. Config + Session Store

Two distinct stores, often confused, kept separate on purpose:

- **Config** — persistent, user-authored settings (theme choice, offline
  mode default, any per-domain preference). Survives across runs. Read by
  any domain; written only through explicit user action, never inferred.
- **Session** — ephemeral, in-memory, TUI-only state (which panel has
  focus, scroll position, last-run command). Discarded on exit. Exists so
  domains don't invent their own ad hoc "current UI state" fields.

```go
// Illustrative — not the final API.
type ConfigStore interface {
    Get(key string) (value string, ok bool)
    Set(key, value string) error // persists to disk
}

type SessionStore interface {
    Get(key string) (value any, ok bool)
    Set(key string, value any) // in-memory only
}
```

## 4. Plugin Host (unchanged scope for now)

The existing JSON-RPC/stdio plugin host (ADR-0002) is **not** part of the
kernel's cross-domain contract in Phase A/B. It stays scoped to Project
Generation. Per ADR-0017, whether it generalizes into a kernel-level
subsystem other domains can use is a Phase C3 decision, not assumed here.

## 5. What the kernel deliberately does not do

- It does not know what a "language," a "vulnerability," or a "test" is —
  those are domain concerns. The kernel only moves opaque models and
  notifications between domains that do know.
- It does not enforce ordering between domains' reactions to the same
  event — if two domains both react to "dependency-added," their handlers
  run independently, and neither may assume the other has finished.
- It is not a database. The Fact Store holds current state only; history
  (e.g. "what changed since 5 minutes ago") is a domain's own concern if
  a domain needs it (Quality's trend view, for instance).

## Phase A2 exit criterion, restated

The kernel (this contract) must compile standalone, have its own test
suite, and have **zero** domain code depending on it yet. That proves the
kernel is decoupled; it deliberately does not yet prove it is useful —
that's what Phase B is for.
