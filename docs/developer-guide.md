# gantry Developer & Contributor Guide

A comprehensive, step-by-step guide for Go developers who want to **contribute to gantry**
or **fork it** and adapt it to their own needs. It covers the repository layout, the
architecture and its guiding principles, the core interfaces, how to extend each pluggable
seam (forge, executor, verifier, notifier, secret backend), the testing strategy, the coding
and commit conventions, the release process, and how to fork responsibly.

It is self-contained. For *operating* gantry, see the [Administrator & Operator
Guide](admin-guide.md); this guide is about the code.

---

## Table of contents

1. [Repository tour](#1-repository-tour)
2. [Architecture & guiding principles](#2-architecture--guiding-principles)
3. [Development environment](#3-development-environment)
4. [The core domain model & interfaces](#4-the-core-domain-model--interfaces)
5. [Request lifecycle: how a `sync` flows through the code](#5-request-lifecycle-how-a-sync-flows-through-the-code)
6. [Extending gantry — the pluggable seams](#6-extending-gantry--the-pluggable-seams)
   - [6.1 Add a forge adapter](#61-add-a-forge-adapter)
   - [6.2 Add an executor](#62-add-an-executor)
   - [6.3 Add a verification probe](#63-add-a-verification-probe)
   - [6.4 Add a notification channel](#64-add-a-notification-channel)
   - [6.5 Add a secret backend](#65-add-a-secret-backend)
7. [Testing strategy](#7-testing-strategy)
8. [Coding conventions](#8-coding-conventions)
9. [Commit, PR & release process](#9-commit-pr--release-process)
10. [Forking & private customization](#10-forking--private-customization)
11. [Contribution workflow checklist](#11-contribution-workflow-checklist)

---

## 1. Repository tour

gantry is a **single Go module** (`github.com/RomanAgaltsev/gantry`), Go 1.26+, with a thin
`main` and all logic under `internal/`.

```
cmd/gantry/main.go        # entrypoint: calls cli.Execute()
internal/
  cli/                    # cobra commands + the composition root (wires adapters to the engine)
  config/                 # gantry.yaml schema, defaulting, validation, SecretRef, durations
  engine/                 # the orchestration core: Sync/Deploy/Promote/Rollback/Switch/Drift/Status
                          #   + the git-backed PinStore (gitstore.go)
  forge/                  # Forge interface + github/ and gitlab/ adapters + a caching wrapper
  executor/               # Executor interface + composessh/ symlinkrelease/ bluegreen/
  verify/                 # Verifier interface + http/compose-ps/command probes
  notify/                 # Notifier interface + webhook/slack/telegram/email + the Dispatcher
  ledger/                 # the append-only deploy-outcome ledger (Entry, Ledger, queries)
  pin/                    # the dotenv pin Set: Read/Render/Diff
  daemon/                 # the serve loop, the single-writer lock, the Observer (metrics) seam
  gitutil/                # git helpers
  humanize/               # human-friendly durations/sizes for reports
  logging/                # slog context helpers (logging.From(ctx))
docs/                     # this documentation set
examples/demo/            # a complete runnable config, deliberately generic
Taskfile.yml              # all dev commands (local == CI)
.github/workflows/        # ci.yml, test.yml, security.yml, pr-title.yml, release.yml
```

Two files worth knowing immediately:

- **`internal/cli/`** is the *composition root*. It reads config, builds the concrete
  adapters (which forge, which executor, which notifiers), constructs the engine, and calls
  a verb. This is where "adding an X" almost always means "adding a `case`."
- **`internal/engine/engine.go`** is the orchestration core. It knows verbs
  (`Sync`, `Deploy`, `Promote`, `Rollback`, `Switch`, …) but **not** which forge/executor
  kind it is talking to — it only sees interfaces.

---

## 2. Architecture & guiding principles

### 2.1 Git is the only state store

There is no database and no server-side state. The engine reads and writes:

- **pin files** (dotenv, one per environment) through the `PinStore` (git-backed);
- the **ledger** (`.gantry/deploys.jsonl`) through the `Ledger`;
- `gantry.yaml` through `config`.

`internal/engine/gitstore.go` implements `PinStore` over `go-git`: `Read`, `ReadAt(sha)`,
`LatestCommit`, `ParentOf`, `Resolve`, `WriteAndCommit`, plus `PullFF`/`Push` for the
fleet-safe daemon topology. Everything else composes on top of those primitives.

### 2.2 The engine never learns the adapter kind

This is the single most important design rule, and it dictates where your code goes. The
engine operates purely on interfaces (`forge.Forge`, `executor.Executor`, `verify.Verifier`,
`ledger.Ledger`, `PinStore`). The mapping from a config `kind` string to a concrete
implementation lives **only** in the `internal/cli` composition root:

```go
// internal/cli/forge.go
// newForge selects the forge adapter by kind. Adding a forge is adding a case here;
// the engine never learns the kind.
```

```go
// internal/cli/executor.go
switch env.Executor.Kind {
case "compose-over-ssh": ...
case "symlink-release":  ...
case "blue-green":       ...
}
```

Consequences for you as a contributor:

- New backends are **additive** — a new `case` plus a new package, no engine change.
- The engine stays testable with fakes and free of transport/registry concerns.
- Capability discovery uses **type assertions**, not `kind` checks. For example, blue-green
  behavior is reached because a `bluegreen` executor also implements
  `executor.SlotExecutor`; a fast rollback is reached because `symlinkrelease` implements
  `executor.FastRollbacker`. The engine asks "does this executor implement the optional
  capability?" rather than "what kind is it?"

### 2.3 Reports vs. logs are separate channels

Reports (what the user asked to see) go to **stdout**; diagnostics go to **stderr** via
`slog` (`internal/logging`, reachable as `logging.From(ctx)`). Never `fmt.Println` a
diagnostic — log it. Never log a report — write it to the command's stdout writer.

### 2.4 Secrets are references, resolved late

No credential is ever an inline literal. `config.SecretRef` is a `${scheme:arg}` string
resolved by a `config.SecretResolver` at use time. The built-in scheme set is fixed and
immutable; a resolver gains extra schemes only through `WithScheme`, which returns a copy
(no shared mutable state).

### 2.5 Safety is enforced in the type/validation layer

Invariants (no inline secrets, required `known_hosts`, unique pin keys, exactly one of
`track`/`promote_from`, a promote target absent from the daemon loop) are enforced in
`config` validation and in the engine's guards — not left to operator discipline. When you
add a feature, add its invariant *here* so it fails loudly and early.

---

## 3. Development environment

### 3.1 Prerequisites

- **Go 1.26+**
- **[Task](https://taskfile.dev)** — `go install github.com/go-task/task/v3/cmd/task@latest`

`task setup` installs the pinned dev tools (golangci-lint, gofumpt, gci) into `./bin`.

### 3.2 Everyday commands

All commands run through `Taskfile.yml` so **local == CI**:

```bash
task            # list available tasks
task setup      # install pinned dev tools into ./bin
task build      # build ./bin/gantry (with version ldflags)
task format     # gofumpt -extra + gci (import grouping)
task vet        # go vet ./...
task lint       # golangci-lint (strict ruleset, .golangci.yml)
task test       # go test -race -shuffle=on ./...
task cover      # coverage profile → coverage.out
task fuzz       # short fuzz smoke of the parsers
task ci         # the full local gate: deps:update, vet, lint, test
```

Run `task ci` before every PR and make sure it is green.

### 3.3 Import grouping

`task format` runs `gci` with three groups: standard, third-party, then
`prefix(github.com/RomanAgaltsev/gantry)`. Keep that ordering; CI checks it.

---

## 4. The core domain model & interfaces

These are the seams you compose against. All live under `internal/`.

### 4.1 `forge` — where releases come from

```go
// internal/forge/forge.go
type Component struct {
    ID      string
    Project string // forge project path or numeric id
    PinKey  string
}

type Release struct {
    Component, SemverVersion, ImageRepository, ImageTag, ImageDigest string
    CommitSHA, ChangelogSection string
    BuiltAt time.Time
}

func (r Release) ImageRef() string // "repo:tag@sha256:…" when a digest is present, else "repo:tag"

type Forge interface {
    LatestRelease(ctx context.Context, c Component) (Release, error)
}
```

A `forge.Cache` decorator (`internal/forge/cache.go`, `NewCache(inner, ttl)`) wraps any
`Forge` so the daemon resolves each component once per cycle.

### 4.2 `executor` — how a host is reconciled

```go
// internal/executor/executor.go
type Plan struct { Env, PinFile string; Pins pin.Set; Commit string }
type Result struct { Changed bool; Detail string }

type Executor interface {
    Deploy(ctx context.Context, p Plan) (Result, error)
}

// Optional capabilities the engine reaches by type assertion:
type SlotExecutor interface {          // blue-green
    Executor
    Slots() (a, b string)
    LiveSlot(ctx context.Context) (string, error)
    SwitchTo(ctx context.Context, slot string) error
}
type FastRollbacker interface {        // symlink-release --fast
    FastRollback(ctx context.Context) (release string, err error)
}
type RunnerCloser interface {          // daemon: release the SSH transport between cycles
    CloseRunner() error
}
```

### 4.3 `verify` — post-deploy health

```go
// internal/verify/verify.go
type Verifier interface { Verify(ctx context.Context) error } // nil error = healthy
type Composite []Verifier                                     // runs each in order, first failure wins

// An executor whose freshly-deployed compose project can be `docker compose ps`-checked:
type ComposeVerifiable interface {
    ComposeTarget(ctx context.Context) (ComposeTarget, error) // blue-green resolves the idle slot
}
```

### 4.4 `notify` — event delivery

```go
// internal/notify/notify.go
type Event struct { Kind, Environment, Commit, By, Message string; Time time.Time }
type Notifier interface { Notify(ctx context.Context, e Event) error }
type Channel struct { Notifier Notifier; Events map[string]bool } // empty Events = all kinds
type Dispatcher []Channel                                          // fans out, best-effort, per-channel timeout
```

Event kinds are constants in one place (`KindDeployed`, `KindPromoted`, `KindRolledBack`,
`KindVerifyFailed`, `KindDriftAlarm`, `KindReconcileFailed`).

### 4.5 `ledger` & `pin`

```go
// internal/ledger/ledger.go
type Entry struct { /* environment, pin_commit, result, healthy, image_set, deployed_at, by */ }
type Ledger interface { /* Record + queries: lookup, latest-green, latest-healthy, history */ }

// internal/pin/pin.go
type Set map[string]string
func Read(r io.Reader) (Set, error)
func Render(s Set) []byte
func Diff(current, desired Set) []Change
```

### 4.6 The engine

```go
// internal/engine/engine_type.go
func New(cfg *config.Config, f forge.Forge, store PinStore, led ledger.Ledger) *Engine
```

Verbs (all in `internal/engine/engine.go` and siblings) take the collaborators they need as
interface parameters: `Sync`, `Deploy`, `Prune`, `Promote`, `Rollback`, `Switch`, `Drift`,
`StatusMatrix`, plus coherence helpers `Orphans` / `MissingKeys`. The `PinStore` is defined
*in the engine package* (`engine.PinStore`) and implemented by `gitStore`.

---

## 5. Request lifecycle: how a `sync` flows through the code

Tracing one verb makes the layering concrete.

1. **`cmd/gantry/main.go`** → `cli.Execute()` runs the cobra tree.
2. **`internal/cli/sync.go`** parses flags (`--env`, `--config`, …), then:
   - `config.Load(path)` reads, defaults, and **validates** `gantry.yaml` (fails before any
     side effect);
   - the composition root builds the concrete **forge** (`newForge`), the **PinStore**
     (`engine.NewGitStore`), the **ledger**, and the environment's **executor**
     (`newExecutor`) and **verifier** (`newVerifier`);
   - `engine.New(cfg, forge, store, ledger)` assembles the engine.
3. **`engine.Sync`**:
   - resolves each forge-tracked component's `LatestRelease` and computes the desired
     `pin.Set`;
   - `pin.Diff(current, desired)` — if empty, it is a no-op (no commit, no deploy);
   - otherwise `WriteAndCommit` the new pin file (commit-on-diff), then `deployAndRecord`:
     the executor's `Deploy`, then the verifier's `Verify`, then `ledger.Record`;
   - on a verify failure with `verify_on_failure: rollback`, it invokes the rollback path
     and stamps `by=auto-rollback`.
4. **notifications** are dispatched best-effort via the `notify.Dispatcher`.
5. results are formatted and written to **stdout**; diagnostics went to **stderr** throughout.

The daemon (`internal/daemon`) reuses the *same* `engine.Sync` on an interval, under the
single-writer lock, reporting each outcome through an `Observer` (the Prometheus seam).

---

## 6. Extending gantry — the pluggable seams

Every extension follows the same shape: **a new package implementing an interface, plus one
`case` in the composition root.** The engine never changes.

### 6.1 Add a forge adapter

Use `internal/forge/gitlab/` and `internal/forge/github/` as templates.

1. **Create the package** `internal/forge/<name>/<name>.go` with a constructor mirroring the
   existing ones:

   ```go
   func New(baseURL, token, marker string, hc *http.Client) *Client
   ```

2. **Implement `forge.Forge`** — one method:

   ```go
   func (c *Client) LatestRelease(ctx context.Context, comp forge.Component) (forge.Release, error)
   ```

   Your job in that method:
   - call the forge API to list releases for `comp.Project`;
   - pick the newest **non-prerelease** (skip any `semver_version` with a SemVer prerelease
     segment; treat empty as stable — reuse the shared metadata parser so this stays
     consistent across forges);
   - parse the release-metadata block delimited by `marker` (the parser is shared —
     `FuzzParseMetadata` covers it) and return a populated `forge.Release`;
   - a **missing or invalid metadata block is a hard error** — never return a zero `Release`
     silently.

3. **Wire it in** `internal/cli/forge.go` — add a `case "<name>":` returning your client.

4. **Reject the unknown kind** — config validation already rejects kinds the switch does not
   handle; add your kind to the accepted set if validation enumerates it.

5. **Tests** — a table-driven test with an `httptest.Server` returning canned release JSON,
   asserting `LatestRelease` resolves the right `ImageRef`, skips prereleases, and errors on
   a bad metadata block.

### 6.2 Add an executor

Templates: `internal/executor/composessh/`, `symlinkrelease/`, `bluegreen/`.

1. **Create the package** and a constructor. If you deploy over SSH, reuse
   `composessh.NewSSHRunner(addr, user, privateKey, knownHosts)` (the `Runner`) rather than
   re-implementing transport, quoting, and host-key pinning.

2. **Implement `executor.Executor`**:

   ```go
   func (e *MyExec) Deploy(ctx context.Context, p executor.Plan) (executor.Result, error)
   ```

   Write the rendered `p.Pins` to the host, pull, and bring the stack up. Honor `p.Commit`
   if your model is versioned (symlink-release names release dirs by it).

3. **Opt into capabilities** by implementing the optional interfaces where they apply:
   - `SlotExecutor` if you have two slots and a switchable pointer (blue-green);
   - `FastRollbacker` if you can revert without a full redeploy;
   - `ComposeVerifiable` (`verify.ComposeVerifiable`) so a `compose-ps` probe can target the
     project you actually deployed to;
   - `RunnerCloser` so the daemon can release your transport between cycles.

   The engine will discover these by type assertion — no engine change needed.

4. **Wire it in** `internal/cli/executor.go` — add a `case "<kind>":` in `newExecutor`,
   reading the executor-specific config fields.

5. **Extend the config schema** in `internal/config` for any new `executor.*` fields, with
   validation (e.g. required sub-fields, mutually-exclusive options).

6. **Tests** — drive `Deploy` against a fake `Runner` that records the commands it was asked
   to run, asserting the exact `docker compose` invocation and the env-file contents.

### 6.3 Add a verification probe

`internal/verify/` holds the probes. A probe is just a `verify.Verifier`.

1. **Implement** `Verify(ctx) error` in a new type (nil = healthy). Host-side probes take a
   `verify.Runner` (SSH); the `http` probe runs from gantry itself.
2. **Wire it in** the verifier builder (`internal/cli/verify.go`) — add a `case` on the probe
   `kind` that constructs your type from its config.
3. **Extend the config** `verify:` item schema for the probe's fields, with validation.
4. **Tests** — assert pass on the healthy path and a descriptive error on failure; if the
   probe parses host output (like `compose-ps`), add a fuzz target for the parser.

### 6.4 Add a notification channel

`internal/notify/fromconfig.go` maps a channel `kind` to a `Notifier`.

- If your channel is "POST JSON to a URL," it is likely a thin variant of the existing
  webhook core (`webhook`/`slack`/`telegram` differ only in payload shape) — add a `case`
  and a payload shaper.
- For a non-HTTP transport (like `email`, see `NewEmailNotifier`), implement `Notifier`
  directly and add a `case`.
- Respect `Channel.Events` filtering (an empty set means "all kinds") and keep sends
  **best-effort** — a channel error must be logged and swallowed, never returned, so a broken
  destination can never fail a deploy. The `Dispatcher` already enforces the per-channel
  timeout.

### 6.5 Add a secret backend

The built-in scheme set (`env`, `file`, `cmd`, `sops`, `vault`, `vault-http`) is fixed. Add a
scheme **per-resolver** via `WithScheme`, which returns a copy with the scheme added:

```go
res := config.DefaultResolver().WithScheme("myvault",
    func(ctx context.Context, r config.SecretResolver, arg string) (string, error) {
        // resolve ${myvault:arg} → secret string; return an error if missing (never "")
    })
```

Keep the invariant: **a referenced-but-missing secret is always an error**, never a silent
empty string. If a scheme shells out to a host binary, bound it with a timeout as the
existing `cmd`/`sops`/`vault` backends do.

---

## 7. Testing strategy

gantry is tested with the standard library plus `stretchr/testify`. `task test` runs
`go test -race -shuffle=on ./...`; keep tests deterministic under shuffling and race.

### 7.1 Layers

- **Unit tests** live beside the code (`*_test.go`). Table-driven is the default style.
- **Fakes over mocks.** Adapters are tested against small hand-written fakes — a fake
  `Runner` recording commands, a fake `Forge` returning canned releases, an in-memory
  `Ledger`/`PinStore`. The interface seams (§4) exist precisely so the engine is testable
  without SSH, a forge, or git.
- **Golden files** anchor formatted report output (the status matrix, history) so a rendering
  change is a visible diff.
- **End-to-end** (`internal/cli/e2e_test.go`) drives whole commands through the cobra tree
  against fakes, asserting stdout/stderr separation and exit behavior.
- **Fuzz targets** guard the parsers — `task fuzz` smoke-tests `pin.Read` (`FuzzRead`), the
  release-metadata parser (`FuzzParseMetadata`), the `compose ps` parser
  (`FuzzParseComposePS`), and shell quoting (`FuzzShellQuote`). Add a fuzz target whenever you
  parse untrusted external text.

### 7.2 What to test when you add a seam

- the happy path returns the expected typed result;
- the invariant fails loudly (bad metadata block, missing secret, empty pin set, unknown
  kind);
- the exact external effect is asserted against a fake (the `docker compose` argv, the
  committed pin-file bytes, the ledger entry written);
- any new parser gets a fuzz target.

Run `task cover` to see where coverage dropped; the CI uploads to Codecov.

---

## 8. Coding conventions

- **Formatting:** `gofumpt -extra` + `gci` (via `task format`). CI enforces both.
- **Linting:** the strict `golangci-lint` ruleset in `.golangci.yml` (v2). `task lint` must
  be clean; prefer fixing the code over a `//nolint` (and justify any `//nolint` you keep).
- **Errors:** wrap with `%w` and context; return them, do not log-and-return. Log at the
  boundary (the CLI/daemon), once. A referenced-but-missing secret, a bad metadata block, and
  an unknown kind are **hard errors**.
- **Context:** every I/O-bound method takes `ctx context.Context` first and honors
  cancellation/timeouts (forge calls, SSH, secret runners).
- **Logging:** structured `slog` through `logging.From(ctx)` to **stderr**; reports to
  **stdout**. Never mix the two.
- **No inline secrets, ever** — not in code, tests, or fixtures. Use `${env:…}`/`${file:…}`.
- **Keep the engine adapter-agnostic** — kind strings live only in `internal/cli` (and config
  validation). If you find yourself switching on a kind inside `engine`, use a capability
  interface and a type assertion instead.
- **Files stay focused** — a growing file is a signal to split by responsibility, matching the
  existing one-purpose-per-file layout in `internal/engine`.

---

## 9. Commit, PR & release process

- **Squash-merge.** The **PR title** becomes the commit on `main` and drives release
  automation, so it **must** be a [Conventional Commit](https://www.conventionalcommits.org/):
  `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`, `build:`, `ci:`, `perf:`. Scope
  optional: `feat(engine): …`. A `pr-title` check enforces this.
- **Breaking changes:** `feat!:` or a `BREAKING CHANGE:` footer. Pre-1.0 this bumps the minor
  version.
- **Before opening a PR:** run `task ci` and make sure it is green.
- **Releasing** is automated by
  [release-please](https://github.com/googleapis/release-please-action): merging
  Conventional-Commit PRs to `main` maintains a standing **release PR** that updates
  `CHANGELOG.md` and the version. Merging that PR tags `vX.Y.Z`, and GoReleaser publishes the
  binaries (see `release.yml`).
- **CI workflows:** `ci.yml` (vet/lint/test), `test.yml`, `security.yml` (CodeQL +
  govulncheck), `pr-title.yml`, `release.yml`. Branch protection requires `lint`, `test`,
  `codeql`, `govulncheck`, and `pr-title` on `main`.

> **Note on gantry's own releases.** gantry publishes GoReleaser binaries and a
> release-please changelog. If you consume gantry *with gantry*, remember the operator-facing
> release-metadata contract (admin guide §8) is a separate concern from how the gantry repo
> itself is released.

---

## 10. Forking & private customization

gantry is MIT-licensed and designed to be forked for a private fleet. Guidance for keeping a
fork healthy:

- **Prefer additive seams over engine edits.** Almost every private need — a bespoke forge, a
  house-specific executor, an internal secret store, a custom notifier — is a new package plus
  a `case`, not a change to `internal/engine`. That keeps you mergeable with upstream.
- **Keep the composition root thin.** New wiring belongs in `internal/cli`; do not thread
  kind-specific logic into the engine.
- **Do not weaken the safety invariants** when adapting for convenience: keep `known_hosts`
  required, keep inline secrets rejected, keep "missing secret is an error." These are the
  properties that make gantry safe to give a deploy account.
- **Track upstream on the interfaces.** The interfaces in §4 are the stable contract; if you
  add optional capability interfaces, mirror the existing "engine discovers by type assertion"
  pattern so an upstream engine can drive your adapter unchanged.
- **Private config stays out of the public repo.** If your fork is public-bound, keep
  environment-specific `gantry.yaml`, host lists, and secrets in a separate private repo — the
  config repo is the trusted boundary (admin guide §19).

If your extension is generally useful (a new mainstream forge, a widely-wanted probe),
consider upstreaming it as a PR instead of carrying a fork.

---

## 11. Contribution workflow checklist

1. Open an issue describing the change (or comment on an existing one) so design is agreed
   before code.
2. Branch, implement following §6/§8, and add tests (§7) — happy path, invariant failure,
   external-effect assertion, and a fuzz target for any new parser.
3. `task ci` green locally.
4. Open a PR with a **Conventional Commit title** (§9); keep the diff focused.
5. Address review; keep the branch rebased. On squash-merge the title lands on `main` and
   feeds release-please.

Thanks for contributing. See the [Administrator & Operator Guide](admin-guide.md) for the
operator's view of everything above.
