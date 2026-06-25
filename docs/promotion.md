# Promotion, rollback, and the deploy-outcome ledger

gantry records every deploy in a git-tracked, append-only ledger at
`.gantry/deploys.jsonl`, keyed by `(environment, pin-file commit SHA)`. Promotion and
rollback are gated on, and recorded in, this ledger.

## The ledger

Each `sync`, `deploy`, `promote`, and `rollback` appends one JSON line:

```json
{"environment":"test","pin_commit":"<sha>","result":"ok","healthy":"unknown","image_set":{"SVC_IMAGE":"reg/svc:v2"},"deployed_at":"2026-06-22T10:00:00Z","by":"sync"}
```

`result` is `ok` or `failed`. `healthy` is `unknown` today; post-deploy health
verification (a later slice) sets it to `true`/`false` and lets promotion require it.

View it per environment:

```bash
gantry history --env test
```

## Promote (test → prod)

`promote` copies the source environment's pin file **as of a specific commit** into the
target environment's pin file, commits it, and deploys the target:

```bash
gantry promote --from test --to prod            # promotes the latest GREEN test deploy
gantry promote --from test --to prod --sha <c>  # promotes a specific commit (must be green)
gantry promote --from test --to prod --dry-run  # show what would happen
```

The set is a **frozen snapshot**: gantry never promotes "the current upstream pin," only
the file as committed at the chosen SHA, so a poller advancing `test` after your decision
cannot leak an unvalidated set into `prod`. Promotion is **gated**: gantry refuses a SHA
whose `test` deploy is missing from the ledger or was not `ok`.

Promotion is wholesale — the full set that was green together moves in one commit.

## Rollback

`rollback` restores an environment's previous pin set (the state at the parent of its
last pin commit), commits it, and redeploys:

```bash
gantry rollback --env prod
gantry rollback --env prod --dry-run
```

Immutable image tags keep the previous images pullable, so rollback is just a forward
deploy of an earlier, already-green set.

## Self-healing sync

If a deploy fails *after* its pin commit, the pins are committed but not running. The
next `gantry sync` notices the latest pin commit has no green ledger entry and redeploys
it automatically (reported as `recovered`). No manual `gantry deploy` is required.

## CI note

gantry commits the pin **and** ledger changes locally; it does not push. In CI the runner
must push these commits so promotion and rollback on another machine see the same history.
```
