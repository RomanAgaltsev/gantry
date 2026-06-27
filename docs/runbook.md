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

## Roll a new release into `test`

```bash
gantry plan --env test     # read-only: show pending pin changes
gantry sync --env test     # pin latest releases, commit, deploy, record
gantry history --env test  # confirm the ok outcome
```

`sync` is a no-op when nothing changed. If a previous deploy failed *after* its pin commit,
`sync` reports `recovered` and redeploys automatically — no manual step needed.

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
