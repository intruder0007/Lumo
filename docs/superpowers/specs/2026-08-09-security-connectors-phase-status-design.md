# Security audit, security-management features, GitHub/SonarQube connectors, phase engine, and `lumo status`

Status: Approved (design). Not yet implemented.

## Context

Lumo (`github.com/intruder0007/Lumo`) is an offline-first, plugin-based
project-scaffolding CLI (see ADR-0001–0016, `docs/architecture/roadmap.md`).
The roadmap already targets a "security/architecture audit" at v0.6.0; this
spec pulls that forward and bundles it with three related asks from the same
request: security-management features for the user/device running Lumo,
outbound connectors to GitHub and SonarQube, and a status UI surfacing all of
it. These four pieces are combined into one spec at the user's explicit
request, overriding the normal recommendation to decompose into separate
specs — flagged once during brainstorming, not re-litigated here.

**Explicit tension with existing principles**: Lumo's README states
"offline first — no network calls required to generate a project" and the
roadmap's non-goals list rules out telemetry and auto-update. The connector
work in this spec is opt-in, outbound-only, and confined to a new `lumo
status` command — it does not touch `lumo new`'s offline guarantee. Every
network call is visibly flagged (Section 2.3).

## Section 1 — Security audit findings and fix

Ran `govulncheck` and `go vet` against `cli` and `core` modules, and checked
`distribution/npm/package.json` for dependency risk, on 2026-08-09.

**Findings:**

- `go vet ./...` clean in both `cli` and `core`.
- `govulncheck` symbol-level scan: 0 vulnerabilities in code actually
  called.
- **GO-2026-5024**: integer overflow in `NewNTUnicodeString`,
  `golang.org/x/sys/windows`, present via `cli`'s indirect dependency on
  `golang.org/x/sys@v0.30.0` (fixed upstream in `v0.44.0`). Not reachable by
  Lumo's own call graph today, but low-risk/zero-cost to fix and
  Windows-relevant given the user base.
- `distribution/npm/package.json` declares zero runtime dependencies —
  nothing to audit.
- No `govulncheck`/`gosec` step exists in `.github/workflows/ci.yml` — a
  process gap, not an active vulnerability.

**Fix plan:**

1. Bump `golang.org/x/sys` to `>= v0.44.0` in `cli/go.mod`, run `go mod
   tidy`, confirm `go build ./...` and existing tests still pass.
2. Add a `govulncheck ./...` step to `.github/workflows/ci.yml` (both `cli`
   and `core` modules) so dependency drift like this is caught
   automatically going forward, not just in one-off audits.

Both are low-risk, mechanical changes — no design decision left open here.

## Section 2 — Security-management features

Four features, each addressing user/device security around the new
network-facing surface this spec adds.

### 2.1 Token storage (SonarQube token; any future GitHub PAT)

No new third-party Go dependency. Lumo already accepts shelling out as the
preferred integration pattern (Section 3's `gh` CLI decision); token storage
follows the same shape — delegate to the OS's native secret store rather
than rolling custom crypto or adding a keyring library:

- **Windows**: `golang.org/x/sys/windows`'s `CryptProtectData`/
  `CryptUnprotectData` (DPAPI) — `x/sys` is already a dependency, so this is
  zero new modules. Encrypted blob stored under the existing
  `os.UserConfigDir()/lumo/` directory.
- **macOS**: shell out to the `security` CLI
  (`add-generic-password`/`find-generic-password`), no new dependency.
- **Linux**: shell out to `secret-tool` (libsecret) if present on `$PATH`.
- **Fallback** (no native store available, e.g. a bare Linux box without
  libsecret): write to a local file at `0600`, and print a one-time warning
  that the value is stored unencrypted-at-rest. Never degrade silently.

The SonarQube **URL** (not secret) stays in the existing plaintext
`prompt.Config` (`lumo config set sonarqube-url`); only the **token** goes
through the secret-store path above (`lumo config set sonarqube-token`,
interactive-only, never accepted as a plaintext CLI flag to avoid shell
history / process-list leakage).

### 2.2 Plugin execution consent

Promotes `SECURITY.md`'s existing claim ("installing a plugin is consent to
run that code") from documentation into a runtime check. Before a plugin
subprocess (template or capability) runs for the **first time** — identified
by `(manifest name, manifest version, resolved binary path)` — Lumo prompts
for confirmation. Approvals are recorded in `prompt.Config` (new field,
append-only set) so subsequent runs of the same plugin version don't
re-prompt. `--yes` and any non-interactive run (per ADR-0007's existing
non-interactive contract: `--answers`, CI, piped stdin) skip the prompt —
this is a UX safeguard for interactive use, not a new hard gate that would
break scripted usage.

### 2.3 Outbound network call visibility

Every GitHub or SonarQube call (Section 3) prints a `→ network: <what and
where>` line to stderr before firing (e.g. `→ network: GitHub API (gh repo
view)`, `→ network: SonarQube (https://sonar.example.com/api/system/status)`).
Suppressed under `--quiet`. This keeps the offline-first default honest:
nothing calls out silently.

`lumo status` additionally gets an `--offline` flag: skip all connector
calls, show only locally-derivable state (repo/Git) plus an explicit
"offline — GitHub/SonarQube not checked" badge for the other two rows,
instead of guessing or caching stale state as if it were live.

### 2.4 Dependency vulnerability gate in `lumo status`

Best-effort: if the current directory is a Go project (`go.mod` present),
`lumo status` runs `govulncheck` against it and folds the summary into the
status view. Follows `core/diag`'s existing best-effort/non-blocking
philosophy — a failed or missing `govulncheck` binary degrades to a
"skipped" row, never blocks the rest of `status`.

**Explicit non-goal for this pass**: non-Go templates (node, python, rust,
etc.) are not covered — no `npm audit`/`pip-audit`/`cargo audit`
integration. Flagged as a known gap, not a silent omission; candidate for a
follow-up spec if needed.

## Section 3 — Connector architecture and phase engine

New `core/connector` package. Connectors are core business logic, not
subprocess plugins — they don't go through `core/plugin`'s JSON-RPC/manifest
machinery, and `SECURITY.md`'s plugin-trust model doesn't apply to them.
Module boundary stays intact: `cli → core → sdk/go`.

### Phase engine

Every connector operation runs as an ordered sequence of named phases:

```
Auth → Fetch → Validate → Display
```

```go
type Phase struct {
    Name string
    Run  func() (Result, error)
}

type Result struct {
    Connected bool
    Detail    string
}
```

The engine runs phases strictly in order and short-circuits on the first
failure, reporting which phase failed (`"sonarqube: failed at auth: token
rejected (401)"`) rather than a bare error. Logs each phase transition
through the existing `core/diag.Logger` seam, so `--verbose` behavior is
free and consistent with how `core/engine`/`core/plugin` already log.

This phase shape is scoped to connector operations only (per explicit
choice during brainstorming) — it is not a general build/rollout-phase
system for other parts of Lumo, and not a project-management delivery plan.

### Connectors

Both are **outbound-only** — Lumo calls out, nothing listens for inbound
webhooks (per explicit choice during brainstorming; a listener would need a
running service, an open port, and inbound auth, which is a materially
larger security surface this spec does not take on).

- **`GitHubConnector`**: shells out to the `gh` CLI (`gh auth status`, `gh
  repo view`). No GitHub token is ever handled directly by Lumo — reuses
  whatever session `gh auth login` already established. If `gh` isn't
  installed, the connector reports "not configured," not an error.
- **`SonarQubeConnector`**: plain HTTP client (Go stdlib `net/http`, no new
  dependency) against a configurable URL — works for self-hosted SonarQube
  or SonarCloud, since both expose the same REST shape. Token comes from
  the Section 2.1 secret store. Calls `/api/system/status` (reachability)
  then a project quality-gate endpoint (validate phase).

Both connectors implement:

```go
type Connector interface {
    Name() string
    Phases() []Phase
}
```

## Section 4 — `lumo status` command

New subcommand (`cli/main.go` gets a `cmdStatus`, alongside `new`/
`plugins`/`config`/`doctor`). Reuses the existing `prompt` theme helpers
(`t.Success`/`t.Failure`/`t.Dim`) already used by `doctor` — no new UI
framework or TUI component needed.

Renders four rows:

1. **Current repo/project** — cwd basename, plus detected
   language/framework if a recognizable project marker (`go.mod`,
   `package.json`, etc.) is present.
2. **Git initialized?** — `.git` directory presence +
   `git rev-parse --is-inside-work-tree`.
3. **GitHub connected?** — `GitHubConnector` result: authenticated via `gh`,
   and (if inside a repo) whether the local remote matches the
   authenticated account.
4. **SonarQube connected?** — `SonarQubeConnector` result, or "not
   configured" if no URL/token is set (never treated as an error — SonarQube
   is opt-in).

Each row is a colored badge (`✅`/`❌`/`➖` not-configured) plus a one-line
detail, matching `doctor`'s existing visual pattern. `--offline` (Section
2.3) skips rows 3–4's network calls and marks them "offline — not checked."
`--verbose` surfaces the phase-by-phase log from Section 3's engine.

## Testing

- **Section 1**: `go build ./...` + existing test suite pass after the
  `x/sys` bump; CI change verified by a green run.
- **Section 2.1**: unit tests for the secret-store abstraction using a fake/
  in-memory backend (matching the existing `core/plugin` test pattern of
  interface + fake, e.g. `host_test.go`); the fallback-file path gets an
  explicit test that it's `0600` and that the warning fires.
- **Section 2.2**: unit test that a first-run plugin triggers the prompt and
  a second run (same manifest name+version+path) doesn't; `--yes`/
  non-interactive bypass covered.
- **Section 3**: phase engine tested with fake phases (success, failure at
  each stage, verifying short-circuit + error message shape) — no real
  network calls in unit tests. `GitHubConnector`/`SonarQubeConnector` get a
  thin integration test behind a build tag or `-short` skip, since they need
  `gh`/a reachable SonarQube instance.
- **Section 4**: golden-output test for `lumo status` in each of: nothing
  configured, everything configured, `--offline`, matching the existing
  `tests/` golden-test convention.

## Non-goals (this spec)

- Inbound webhooks / any listening service (deferred, larger surface).
- Non-Go dependency vulnerability scanning (Section 2.4 gap).
- A general-purpose build/rollout phase system beyond connector operations.
- GitHub PAT direct-auth path (Section 2's `gh`-CLI decision covers this
  spec; revisit only if `gh`-CLI-only proves insufficient in practice).
