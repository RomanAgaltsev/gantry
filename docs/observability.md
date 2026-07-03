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
