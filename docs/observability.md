# Observability

gantry has two output channels, kept deliberately separate:

- **Reports** (what you asked to see) go to **stdout**.
- **Logs** (what gantry did, for diagnostics) go to **stderr**.

So `gantry status --all > matrix.txt` captures only the matrix, and
`gantry sync --env test --log-format json 2> gantry.log` captures only the
diagnostics.

## The status matrix

`gantry status --all` prints every component (rows) against every environment
(columns), with a `latest` column from the forge:

```
COMPONENT   latest      test         prod
SVC_IMAGE   reg/svc:v9  reg/svc:v9   reg/svc:v8 !
PG_IMAGE    (untracked) postgres:16.2 postgres:16.2

test  ok      healthy   2h ago
prod  (no deploys)
```

- A `!` after a cell means that environment's pin lags the latest release
  (tracked components only; explicit/registry-sourced components show
  `(untracked)` and are never marked).
- The footer shows each environment's most recent deploy outcome from the
  ledger: result, health, and age. An environment with no deploys yet prints
  `(no deploys)`.

`gantry status --env <name>` keeps the single-environment list.

## Logging

Two persistent flags control the diagnostic log on stderr:

- `--log-format text|json` (default `text`)
- `--log-level debug|info|warn|error` (default `info`)

Logs are structured (`slog`). Use `--log-format json` when shipping logs to a
collector. The daemon exposes Prometheus metrics — see
[daemon.md#metrics](daemon.md#metrics); a short-lived CLI run has nothing to scrape.

## Example Prometheus alert rules

The daemon exports the counters and gauges listed in
[daemon.md#metrics](daemon.md#metrics). The rules below are a starting point;
adjust thresholds to your fleet.

```yaml
groups:
  - name: gantry
    rules:
      - alert: GantryDriftStuck
        # Fires while a component's latest release is unconsumed. The daemon writes
        # gantry_drift_age_seconds every pass and resets it to 0 when drift resolves, so
        # this alert auto-resolves once the pin catches up.
        expr: gantry_drift_age_seconds > 86400
        for: 30m
        labels: { severity: warning }
        annotations:
          summary: "gantry drift on {{ $labels.env }} > 24h"

      - alert: GantryVerifyFailures
        expr: increase(gantry_verify_failures_total[15m]) > 0
        labels: { severity: warning }
        annotations:
          summary: "gantry post-deploy verify failing on {{ $labels.env }}"

      - alert: GantryReconcileStalled
        # No successful reconcile recently (a deploy or a no-change pass both count).
        expr: increase(gantry_reconcile_total{result="deployed"}[1h]) == 0 and increase(gantry_reconcile_total{result="nochange"}[1h]) == 0
        for: 1h
        labels: { severity: critical }
        annotations:
          summary: "gantry reconcile loop appears stalled"
```

`GantryDriftStuck` relies on the gauge resetting to 0 on resolve: that behaviour
shipped with the drift-observe fix (the gauge is written every pass, not only
when drift is found), so the alert clears as soon as the environment's pin
catches up to the latest release.
