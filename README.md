# gantry

[![ci](https://github.com/RomanAgaltsev/gantry/actions/workflows/ci.yml/badge.svg)](https://github.com/RomanAgaltsev/gantry/actions/workflows/ci.yml)
[![test](https://github.com/RomanAgaltsev/gantry/actions/workflows/test.yml/badge.svg)](https://github.com/RomanAgaltsev/gantry/actions/workflows/test.yml)
[![security](https://github.com/RomanAgaltsev/gantry/actions/workflows/security.yml/badge.svg)](https://github.com/RomanAgaltsev/gantry/actions/workflows/security.yml)
[![codecov](https://codecov.io/gh/RomanAgaltsev/gantry/branch/main/graph/badge.svg)](https://codecov.io/gh/RomanAgaltsev/gantry)
[![Go Reference](https://pkg.go.dev/badge/github.com/RomanAgaltsev/gantry.svg)](https://pkg.go.dev/github.com/RomanAgaltsev/gantry)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**gantry** is a non-Kubernetes release orchestrator. It reads the latest published
release of each of your components from a forge (GitLab or GitHub), pins the resolved
immutable image references into per-environment dotenv files committed to git, deploys
over SSH (plain compose, symlink-release, or blue-green), verifies health, records every
outcome in a git-tracked deploy ledger, and gates promotion on that ledger. It runs
one-shot from CI or continuously as a daemon with Prometheus metrics and a webhook
doorbell.

## The gap it fills

Plenty of teams run their services with Docker Compose on plain hosts rather than
Kubernetes. They still want what Kubernetes-style GitOps gives you: a single declarative
description of which version runs where, an auditable history of every version bump, a
verified path from "a new release exists" to "it is running in test", and a gated
promotion to production that can only ship a revision a green test deploy has already
proven. gantry provides exactly that for the Compose-over-SSH world — git is the only
state store, and there is no system-specific logic baked in.

## How it works

- **Git is the source of truth.** Pins are dotenv files in git; the deploy ledger is
  JSONL in git; the daemon is stateless (each loop reads git). Crash-safety, auditability,
  and `git log` as the debugging UI all fall out for free.
- **The deploy ledger is the promotion gate.** Every deploy records its outcome keyed by
  `(environment, pin-commit)`. Promotion to prod reads that ledger and refuses any revision
  without a green (optionally *healthy*) test deploy — "prod requires a proven revision" is
  enforceable, not a convention.
- **Promotion is frozen.** `promote` reads the pin file *as committed at the gated SHA*,
  never "current upstream", closing the TOCTOU race between validating and shipping.
- **Rollback targets the last green ledger entry**, not the parent commit, so repeated
  rollbacks walk backward through known-good states instead of oscillating onto a bad one.

## Capabilities

- **Forges:** GitLab and GitHub Release adapters; both resolve "latest" to the newest
  non-prerelease release and parse an embedded release-metadata block
  (`repository:tag`, digest, commit, changelog). With a digest the pin is written as
  `repository:tag@sha256:…` so a re-pushed tag cannot drift the pulled image.
- **Executors:** `compose-over-ssh` (`docker compose pull && up -d`), `symlink-release`
  (atomic `current` symlink flip over per-release dirs), and `blue-green` (stage the idle
  slot, gate the switch on a green ledger entry plus a fresh idle-slot verify).
- **Verification:** post-deploy `http`, `compose-ps`, and `command` probes; an optional
  `verify_on_failure: rollback` auto-rolls-back a failed verify (held for blue-green, whose
  pointer is the safety mechanism).
- **Promotion / rollback / drift:** gated `promote`, ledger-targeted `rollback`, and a
  `drift` report (exit code 3) for tracked components whose latest release is unconsumed
  past a threshold.
- **Status matrix:** `status --all` shows latest-vs-pinned per component across every
  environment plus each environment's health.
- **Notifications:** `webhook`, `slack`, `telegram`, and `email` channels for
  deploy/promote/rollback/verify/drift/reconcile-failed events, per-channel event filtering.
- **Daemon (`serve`):** a reconcile loop under a single-writer lock, with `/healthz`,
  Prometheus `/metrics`, and an optional authenticated webhook **doorbell** that triggers an
  immediate (debounced) reconcile.
- **Secrets:** every credential is a `${scheme:…}` reference — `env`, `file`, `cmd`, `sops`,
  `vault` — resolved at use; inline literals are rejected.

## Quickstart

```bash
task build            # or: go build -o gantry ./cmd/gantry

export GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx   # referenced as ${env:GANTRY_FORGE_TOKEN}

# Inspect, then apply
./gantry plan    --env test --config gantry.yaml
./gantry sync    --env test --config gantry.yaml
./gantry status  --all       --config gantry.yaml

# Gated promotion and rollback
./gantry promote --from test --to prod --config gantry.yaml
./gantry rollback --env prod  --config gantry.yaml

# Run continuously (daemon) with metrics on :9713
./gantry serve --config gantry.yaml
```

A complete, runnable configuration lives in [`examples/demo`](examples/demo/) — a generic
two-component setup, intentionally unrelated to any specific system, to show gantry is
driven entirely by `gantry.yaml`.

## Exit codes

`0` success · `3` drift detected (distinct from failure, a CI affordance) · `1` any other
operational error.

## Documentation

- [Getting started](docs/getting-started.md) · [Configuration reference](docs/configuration.md)
- [Executors](docs/executors.md) · [Blue-green](docs/blue-green.md) · [Verification](docs/verification.md)
- [Promotion](docs/promotion.md) · [Drift](docs/drift.md)
- [Daemon](docs/daemon.md) · [Observability](docs/observability.md)
- [Secrets](docs/secrets.md) · [Notifications](docs/notifications.md) · [Runbook](docs/runbook.md)

## License

Released under the [MIT License](LICENSE).
