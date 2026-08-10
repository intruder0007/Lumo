# Security Audit Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the one dependency vulnerability found in the 2026-08-09 security audit and wire `govulncheck` into CI so future drift is caught automatically.

**Architecture:** Two independent, mechanical changes: a `go.mod` dependency bump in `cli`, and one new CI step in the existing `.github/workflows/ci.yml` pipeline.

**Tech Stack:** Go modules, GitHub Actions.

## Global Constraints

- Go 1.22 (matches `go.work`, all `go.mod` files, and the CI `setup-go` step).
- No new third-party dependencies — this plan only bumps an existing indirect dependency.
- CI must stay green end-to-end after both changes (existing `Build & vet present modules` / `Test present modules` / `make build` / markdownlint steps).

---

### Task 1: Bump `golang.org/x/sys` past GO-2026-5024

**Files:**
- Modify: `cli/go.mod`
- Modify: `cli/go.sum`

**Interfaces:**
- Consumes: nothing from other tasks (this plan's tasks are independent of each other).
- Produces: `cli`'s `golang.org/x/sys` requirement at `>= v0.44.0`, consumed by nothing else in this plan.

- [ ] **Step 1: Confirm the current vulnerable version and reproduce the finding**

Run: `cd cli && go run golang.org/x/vuln/cmd/govulncheck@latest ./...`

Expected output includes:

```
Vulnerability #1: GO-2026-5024
    Invoking integer overflow in NewNTUnicodeString in golang.org/x/sys/windows
  Module: golang.org/x/sys
    Found in: golang.org/x/sys@v0.30.0
    Fixed in: golang.org/x/sys@v0.44.0
```

- [ ] **Step 2: Bump the dependency**

Run: `cd cli && go get golang.org/x/sys@v0.44.0 && go mod tidy`

- [ ] **Step 3: Verify the finding is gone**

Run: `cd cli && go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
Expected: no `Vulnerability #1` block for `GO-2026-5024` (only "No vulnerabilities found" / an unrelated future finding, if any).

- [ ] **Step 4: Verify the module still builds and its tests pass**

Run: `cd cli && go build ./... && go vet ./... && go test ./...`
Expected: all pass, no new errors introduced by the bump.

- [ ] **Step 5: Commit**

```bash
git add cli/go.mod cli/go.sum
git commit -m "fix: bump golang.org/x/sys to v0.44.0 (GO-2026-5024)"
```

---

### Task 2: Add `govulncheck` to CI

**Files:**
- Modify: `.github/workflows/ci.yml:130-131` (insert a new step between the existing `Test present modules` step, ending at line 130, and the `Build via Makefile` step, starting at line 141)

**Interfaces:**
- Consumes: nothing (independent of Task 1 — this step would have caught Task 1's finding, but doesn't depend on the fix already existing to be added).
- Produces: nothing consumed elsewhere in this plan; this is CI configuration only.

- [ ] **Step 1: Add the CI step**

Insert immediately after the `Test present modules` step (after line 130, before the blank line and the `make build` step) in `.github/workflows/ci.yml`:

```yaml
      # Catches dependency-level vulnerabilities (e.g. GO-2026-5024) that
      # `go vet`/`go test` don't check for. Scoped to cli and core only —
      # sdk/go, templates/*, and plugins/builtin/* are checked separately
      # if/when they grow their own third-party dependencies; today they
      # have none.
      - name: Vulnerability scan (govulncheck)
        run: |
          for m in cli core; do
            echo "== $m =="
            (cd "$m" && go run golang.org/x/vuln/cmd/govulncheck@latest ./...)
          done
```

- [ ] **Step 2: Validate the YAML is well-formed**

Run: `npx --yes js-yaml .github/workflows/ci.yml > /dev/null && echo OK`
Expected: `OK`, no parse error.

- [ ] **Step 3: Run the same commands locally to confirm the step would pass**

Run: `for m in cli core; do (cd "$m" && go run golang.org/x/vuln/cmd/govulncheck@latest ./...); done`
Expected: both modules report "Your code is affected by 0 vulnerabilities" (after Task 1's bump; if Task 1 hasn't landed yet, `cli` still reports the symbol-level "0 vulnerabilities" result — the package-level GO-2026-5024 note doesn't fail the command, so this step doesn't hard-block on Task 1, but should be run after Task 1 in practice so the workflow file and the dependency state move together).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add govulncheck scan for cli and core modules"
```
