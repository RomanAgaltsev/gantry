# The `gantry serve` daemon

`gantry serve` runs the reconcile loop as a long-lived process: it calls the same
`engine.Sync` the one-shot `gantry sync` does, on an interval, across every
**track-mode** environment, under a single-writer lock. It is how gantry keeps an
environment pinned to the latest releases without a CI scheduler triggering each run.

The engine is unchanged — `serve` builds the collaborators once and reuses the same
verbs the CLI does. A reconcile error is logged, observed, and notified; it **never**
stops the loop. `Run` returns only when the process is interrupted.

## Configure it

`daemon:` is an optional top-level block — every field defaults, so an existing config
runs the daemon unchanged:

```yaml
daemon:
  interval: 60s    # reconcile period; minimum 1s
  listen:  ":9713" # HTTP bind address for /healthz
  reconcile_timeout: 5m # per-environment reconcile deadline; minimum 1s
  reconcile_failed_repeat: 1h # suppress repeat reconcile_failed alerts per environment
  doorbell:        # optional forge-webhook trigger — see "Doorbell" below (C3c)
    enabled: false
```

| field | default | notes |
| --- | --- | --- |
| `interval` | `60s` | How often each track-mode environment is reconciled. Must be ≥ `1s`. Override once with `gantry serve --interval 30s`. |
| `listen` | `:9713` | HTTP address `/healthz` and `/metrics` are served on. |
| `reconcile_timeout` | `5m` | Per-environment deadline for one reconcile. A wedged remote command (e.g. a stuck `docker compose pull`) fails that env's reconcile after the timeout instead of blocking the loop; `/healthz` keeps answering `ok`. Must be ≥ `1s`. |
| `reconcile_failed_repeat` | `1h` | A failing environment emits one `reconcile_failed` notification per window (so a flapping host doesn't spam); the first success after a failing streak emits a `deployed` recovery note. |
| `doorbell` | disabled | Trigger an immediate reconcile from a forge webhook instead of waiting for the next tick. See [Doorbell](#doorbell). |

Notifications are configured with the existing top-level [`notifications:`](notifications.md)
block — `serve` dispatches the same events the CLI does (`deployed`, `rolled_back`,
`verify_failed`, `drift_alarm`) on every reconcile, best-effort.

## What it reconciles

Only environments with `source: { track: ... }` are auto-reconciled (C3-D8). A
**promote-mode** environment (`source: { promote_from: ... }`) is **never** touched by the
loop — promotion stays a deliberate, human/CI-driven act via `gantry promote`. See
[promotion.md](promotion.md).

Each tick, for every track-mode environment, `serve`:

1. resolves the latest releases from the forge,
2. commits any pin diff (commit-on-diff — a no-op when nothing changed),
3. deploys over SSH and runs the environment's `verify:` probes,
4. records the outcome in the ledger, and
5. dispatches notification events.

A per-environment executor that fails to build (e.g. a missing SSH secret) is **skipped**
for that environment on that tick — the loop keeps going; the failure is logged. This
matches [verification.md](verification.md): a `verify_on_failure: rollback` environment
auto-reverts to its last known-good set inside the loop, just as in CLI mode.

## The single-writer lock

`serve` takes an advisory lock at `<repo>/.gantry/serve.lock` before it starts looping,
holding the owner's PID and start time. While a fresh lock is held, the mutating CLI verbs
refuse to run:

```
$ gantry sync --env test
Error: a gantry daemon is reconciling this repo (.gantry/serve.lock); retry when it is stopped
```

This prevents the daemon and a one-off `sync`/`deploy`/`promote`/`rollback`/`switch` from
writing the pin file and deploying concurrently. A lock whose owner process is dead (or
older than 24h) is treated as stale and reclaimed, so a crashed daemon does not strand the
repo.

## Topology: one writer clone, or `git.remote` sync

The lock is **per-filesystem** — it does not coordinate across machines. The supported
default is **one writer clone per repo**: exactly one place runs `gantry serve` (and the CLI
verbs against that clone), period. Running two daemons against two clones of the same repo
with no coordination splits the ledger and pin history by default.

When the daemon must run somewhere other than the only clone (e.g. it lives on a deploy host
while operators and CI work from their own clones), turn it into a fleet-safe worker with the
`git.remote` block:

```yaml
git:
  remote:
    name: origin      # default origin
    branch: main      # optional; defaults to the current HEAD branch
    pull: true        # fast-forward pull at the top of each reconcile cycle
    push: true        # push after each cycle that committed
    username: gantry  # HTTPS basic-auth username (token name); optional
    token: ${env:GANTRY_GIT_TOKEN}  # required when pull/push is enabled (HTTPS auth)
```

With `pull: true`, the daemon fast-forward-pulls origin at the top of every cycle before any
reconcile, so a Renovate bump to an explicit pin (or another clone's commit) arriving via
origin is seen on the next tick. With `push: true`, it pushes after any cycle that committed
its own pin change. gantry **never merges**: a non-fast-forward pull (the clones diverged) is
a loud stop — it logs a `reconcile_failed` alert and skips that cycle's writes rather than
creating a divergent merge. SSH remotes authenticate via the user's SSH agent and can leave
`token` unset (the validation requires it whenever pull/push is on, since gantry cannot see
the remote URL from config; SSH transport ignores it).

## `/healthz` and shutdown

`/healthz` returns `200 ok` while the daemon is running — point a load balancer or uptime
check at it:

```bash
curl http://127.0.0.1:9713/healthz   # → ok
```

`SIGINT` (`Ctrl-C`) and `SIGTERM` trigger a graceful shutdown: the reconcile loop stops, the
HTTP server is given 5s to drain, and the lock is released. In-flight reconcile calls are
bounded by `daemon.reconcile_timeout`; a stuck host fails that environment's reconcile after
the timeout rather than wedging the loop, and `/healthz` keeps answering `ok` throughout.

## Metrics

The same HTTP server exposes Prometheus metrics at `/metrics` (on `daemon.listen`, shared
with `/healthz`):

```bash
curl http://127.0.0.1:9713/metrics
```

The daemon records every reconcile outcome through its `Observer`; the families are:

| metric | type | labels | meaning |
| --- | --- | --- | --- |
| `gantry_reconcile_total` | counter | `env`, `result` | Reconcile passes by result: `deployed`, `failed`, or `nochange`. |
| `gantry_reconcile_duration_seconds` | histogram | `env` | Wall-clock time of one reconcile. |
| `gantry_deploys_total` | counter | `env` | Actual deploys performed (a reconcile with a real diff). |
| `gantry_verify_failures_total` | counter | `env` | Post-deploy verify probes that failed. |
| `gantry_rollbacks_total` | counter | `env`, `kind` | Rollbacks performed; `kind="auto"` is a verify-triggered auto-rollback. |
| `gantry_drift_age_seconds` | gauge | `env` | Age (seconds) of the oldest drifted component, last observed. |
| `gantry_build_info` | gauge | `version` | Constant `1`, carrying the gantry version label. |

A scrape config for Prometheus (the job targets the daemon's listen port):

```yaml
scrape_configs:
  - job_name: gantry
    static_configs:
      - targets: ["localhost:9713"]
```

`gantry_drift_age_seconds` reflects the last reconcile's finding for each environment; see
[drift.md](drift.md) for the drift model.

## Running it under systemd

Run `serve` as a service on the orchestrator host (the machine with the git repo and SSH
access to the deploy targets). It commits locally like every gantry verb; push the pin and
ledger commits separately if other machines need to see them.

```ini
# /etc/systemd/system/gantry.service
[Unit]
Description=gantry reconcile daemon
After=network-online.target

[Service]
Type=simple
User=gantry
WorkingDirectory=/srv/gantry       # the orchestrator git repo (holds gantry.yaml)
Environment=GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
ExecStart=/usr/local/bin/gantry serve --config /srv/gantry/gantry.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`Restart=on-failure` covers a crash; `WorkingDirectory` (or `--config`) must point at the
repo. Stop it with `systemctl stop gantry` (a `SIGTERM`).

## Doorbell

A forge webhook can trigger an immediate reconcile instead of waiting for `interval` to
elapse. Enable it under `daemon.doorbell`:

```yaml
daemon:
  doorbell:
    enabled: true
    path: /hooks/forge        # default; the URL the forge POSTs to
    secret: ${env:GANTRY_DOORBELL_TOKEN}  # required when enabled; authenticates the webhook
```

The webhook **carries no version data** (C3-D2) — it only says "something changed, go look".
An authenticated ring schedules the same reconcile the interval would run; it cannot cause a
deploy directly. gantry authenticates the request with a shared secret sent in either the
`X-Gantry-Token` or the `X-Gitlab-Token` header (constant-time compare). Bursts are
debounced: the doorbell is a capacity-1 channel, so a flurry of webhooks collapses to a
single pending reconcile.

To wire it up in GitLab, add a webhook to the relevant project(s) pointing at gantry with the
secret token set to the configured value:

- **URL:** `http://<host>:9713/hooks/forge`
- **Secret token:** the value `GANTRY_DOORBELL_TOKEN` resolves to
- **Trigger:** `Release` events (a push/mirror will do)

Test it by hand (expect `202 Accepted`):

```bash
curl -XPOST -H "X-Gantry-Token: $GANTRY_DOORBELL_TOKEN" http://host:9713/hooks/forge
```

The endpoint authenticates by shared secret, so in production it should sit behind TLS or an
ingress that terminates TLS for the gantry port.

## What is *not* here yet

The C3 daemon slices (core loop, metrics, doorbell) are complete. GitHub HMAC signing
(`X-Hub-Signature-256`) is a documented seam in the doorbell's authenticator rather than
built today — the shared-secret header covers GitLab natively and any webhook that can set a
header; add HMAC when a GitHub adopter needs it.
