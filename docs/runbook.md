# gantry operations runbook

Day-to-day operating procedures for an environment that gantry manages. For the
conceptual model of the ledger, promotion, and rollback, see
[promotion.md](promotion.md).

## Preconditions

- gantry runs **inside the orchestrator git repo** (the one holding `gantry.yaml` and the
  `.env.versions.<env>` pin files) and commits to it.
- gantry **must own the working tree**: it builds each commit from the git index, so it
  refuses to act when unrelated changes are already staged
  (`refusing to commit: "<path>" is already staged`). Commit or unstage them first.
- gantry commits locally and does **not** push. In CI the runner must push the pin **and**
  ledger commits so promotion/rollback on another machine see the same history.
- credentials are **SecretRefs** (`${env:…}`/`${file:…}` built in; `${cmd:…}`, `${sops:…}`,
  `${vault:…}` shell out to host CLIs). Point a credential at Vault/SOPS with e.g.
  `password: ${sops:secrets.enc.yaml#reg.password}` — but the matching `sops`/`vault` binary
  must be present on the host (the default distroless image ships only `env`/`file`/`cmd`).
  A referenced-but-missing secret always errors; see [secrets.md](secrets.md).

## Roll a new release into `test`

```bash
gantry plan --env test     # read-only: show pending pin changes
gantry sync --env test     # pin latest releases, commit, deploy, record
gantry history --env test  # confirm the ok outcome
```

`sync` is a no-op when nothing changed. If a previous deploy failed *after* its pin commit,
`sync` reports `recovered` and redeploys automatically — no manual step needed.

`plan` also reports **orphan pins** — keys present in the pin file but no longer declared in
`gantry.yaml` (left behind when a component is deleted or its `pin_key` renamed). Remove them
and drop the now-unwanted containers with:

```bash
gantry prune --env test --dry-run   # list the orphan keys it would remove
gantry prune --env test             # remove them, commit, and redeploy the reduced set
```

`prune` reuses the normal write-commit-deploy path and refuses to prune a set down to empty
(that would deploy with no images at all). Removing the component from `gantry.yaml` and
running `prune` is the supported way to retire a component.

## Detect drift

```bash
gantry drift --env test   # one environment
gantry drift --all        # every track-mode environment (CI gate)

## Promote `test` → `prod`

```bash
gantry promote --from test --to prod --dry-run   # preview (prints the gate decision)
gantry promote --from test --to prod             # promote the latest GREEN test set
gantry promote --from test --to prod --sha <c>   # promote a specific commit (short SHA ok)
```

The set is a frozen snapshot of the source pin file as committed at the chosen SHA, gated
on a green ledger entry. If you promote against an edge other than the target's configured
`promote_from`, gantry prints a warning but proceeds.

## Roll `prod` back

```bash
gantry rollback --env prod --dry-run   # preview the target set
gantry rollback --env prod             # restore the last known-good set and redeploy
```

Rollback targets the most recent `ok` ledger entry older than the current pin commit, so it
never redeploys a set the ledger recorded as `failed`. Run it again to step further back
through good states. It refuses when there is no earlier green deploy on record.

## Blue/green: stage and switch

```bash
gantry sync   --env front   # stage the new version on the idle slot (live slot untouched)
gantry switch --env front   # flip the pointer to the staged slot (gated on an ok deploy)
gantry rollback --env front # flip back to the previous slot
```

`switch` refuses if the idle slot's last deploy is not `ok`. Both `switch` and `rollback`
are recorded in the ledger (`by: switch` / `by: rollback`). See
[blue-green.md](blue-green.md).

## Run the daemon continuously

`gantry serve` runs the reconcile loop as a long-lived process — it runs `sync` for you
on an interval, across every track-mode environment, so `test` stays pinned to the latest
releases without a CI schedule:

```bash
gantry serve               # reconcile every 60s (default) until interrupted
gantry serve --interval 15s
```

- **Scope:** only **track-mode** environments are auto-reconciled; promote targets
  (`prod`) are never advanced by the loop — keep running `gantry promote` by hand/CI.
- **Stop it:** `Ctrl-C` or `SIGTERM` (`systemctl stop gantry` under systemd). It shuts
  down gracefully: the loop stops, `/healthz` drains, and the lock is released.
- **Lock:** while it runs it holds `.gantry/serve.lock`, and the mutating verbs
  (`sync`/`deploy`/`promote`/`rollback`/`switch`) refuse with
  *"a gantry daemon is reconciling this repo"*. A stale lock (dead owner, or >24h old) is
  reclaimed automatically.
- **Health:** `/healthz` returns `ok` on the listen address (default `:9713`). A
  stuck remote host (e.g. a wedged `docker compose pull`) no longer blocks the whole loop:
  that environment's reconcile fails after `daemon.reconcile_timeout` (default `5m`) and the
  loop keeps reconciling the others, with `/healthz` still answering `ok`.
- **Metrics:** scrape `/metrics` on the same port. Watch `gantry_drift_age_seconds` (a
  component lagging its release past your threshold) and `gantry_verify_failures_total`
  rising (reconciles that deployed but failed verification).
- **Doorbell:** point a forge webhook at `http://<host>:9713/hooks/forge` with the shared
  secret in `X-Gantry-Token` (or `X-Gitlab-Token`) to reconcile immediately on a release
  instead of waiting for the next tick. Enable it under `daemon.doorbell`. Test by hand:
  `curl -XPOST -H "X-Gantry-Token: …" host:9713/hooks/forge` → `202`.

See [daemon.md](daemon.md) for config, the metrics families, the doorbell, and the systemd
unit.

## A deploy failed — now what?

A `sync`/`promote`/`rollback` whose deploy fails *after* its pin commit leaves the pins
committed but not running. gantry records the outcome as `failed` and tells you so.

- **`test` (track mode):** rerun `gantry sync --env test` — it self-heals (redeploys the
  committed set and records a fresh outcome).
- **`prod` (promote target):** the CLI prints a hint —
  `run gantry deploy --env prod to retry`. `gantry deploy` reconciles the host to the
  committed pin file and records the new outcome.

Inspect what happened at any time:

```bash
gantry history --env prod
```

Each line is one outcome: timestamp, `ok`/`failed`, `healthy`, `by` (sync/deploy/
promote/rollback), and the pin commit SHA.

- **See everything at a glance:** `gantry status --all` — the cross-environment
  version + health matrix (reads forge + pin files + ledger; resolves no SSH or
  registry secrets).
