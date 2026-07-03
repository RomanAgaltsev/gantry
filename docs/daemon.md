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
  doorbell:        # optional forge-webhook trigger — see "Doorbell" below (C3c)
    enabled: false
```

| field | default | notes |
| --- | --- | --- |
| `interval` | `60s` | How often each track-mode environment is reconciled. Must be ≥ `1s`. Override once with `gantry serve --interval 30s`. |
| `listen` | `:9713` | HTTP address `/healthz` is served on. |
| `doorbell` | disabled | Trigger a reconcile on a forge webhook instead of waiting for the next tick. This is a C3c feature and not yet active; setting it has no effect in C3a. |

Notifications are configured with the existing top-level [`notifications:`](notification.md)
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

## `/healthz` and shutdown

`/healthz` returns `200 ok` while the daemon is running — point a load balancer or uptime
check at it:

```bash
curl http://127.0.0.1:9713/healthz   # → ok
```

`SIGINT` (`Ctrl-C`) and `SIGTERM` trigger a graceful shutdown: the reconcile loop stops, the
HTTP server is given 5s to drain, and the lock is released. In-flight reconcile calls run to
completion; a reconcile is never interrupted mid-deploy.

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

## What is *not* here yet

- **Metrics** (Prometheus) arrive in C3b. The reconcile loop already records outcomes
  through an `Observer` seam (a no-op today); C3b swaps in a real implementation and
  exposes `/metrics` on the same HTTP mux as `/healthz`.
- **Doorbell** (forge-webhook-triggered reconcile) is C3c. The `Options.Doorbell` channel
  and the `daemon.doorbell` config block are placeholders; a nil doorbell means the loop is
  interval-only.
