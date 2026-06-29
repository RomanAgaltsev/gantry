# Health verification

After a successful deploy, gantry can run **verification probes** to confirm the
environment is actually healthy — not merely that `docker compose up` exited 0. The
result is recorded in the deploy-outcome ledger's `healthy` field, and the promotion gate
can be told to require it.

## Probes

Configure one or more probes per environment under `verify:`. **All must pass.**

```yaml
environments:
  - name: prod
    # …executor…
    verify:
      - { kind: http, url: https://app.example.com/healthz, expect_status: 200 }
      - { kind: compose-ps }
      - { kind: command, command: "test -f /opt/app/.ready" }
```

| kind | runs from | passes when |
| --- | --- | --- |
| `http` | gantry | `GET url` returns `expect_status` (default 200) |
| `compose-ps` | the host | every compose service is running, and healthy if it declares a healthcheck |
| `command` | the host | the command exits 0 |

## What a failed probe does

A failed probe records the outcome as `result: failed, healthy: false`, prints the error,
and exits non-zero. The stack is **left as deployed** — gantry does not auto-roll-back
(that is a future opt-in). To recover:

- **track-mode (`sync`):** rerun `gantry sync --env <e>` — it redeploys and re-verifies.
- **promote target:** fix the cause and `gantry deploy --env <e>` to retry, or
  `gantry rollback --env <e>`.

A failed verification also makes the revision un-promotable when the gate requires health
(below).

## Requiring health to promote

```yaml
promote:
  require_healthy: true   # default false
```

With `require_healthy: true`, `gantry promote` only accepts a source revision whose ledger
entry is `ok` **and** `healthy: true`. Default (`false`) keeps the A2 behavior (a green
`ok` entry is enough). See [promotion.md](promotion.md).
