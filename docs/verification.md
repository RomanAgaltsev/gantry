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

## Auto-rollback on failed verify

By default a failed post-deploy verify is recorded as `failed`/`unhealthy`, the command exits
non-zero, and the broken deploy is left in place for inspection (`verify_on_failure: hold`).

Set `verify_on_failure: rollback` on an environment to automatically revert to its last
known-good pin set when a verify fails:

```yaml
environments:
  - name: test
    verify:
      - { kind: compose-ps }
    verify_on_failure: rollback
```

- Applies to `sync`, `deploy`, and `promote` (a failed `promote` reverts the *target*
  environment). `rollback` and `switch` never auto-roll-back.
- The revert reuses `gantry rollback`; its ledger entry is stamped `by=auto-rollback`, so
  `gantry history` shows why the environment reverted.
- The command still exits non-zero — auto-rollback restores service, it does not hide the
  failure.
- `rollback` requires at least one `verify` probe (otherwise nothing can fail).

Note: because `sync` commits the new pin before deploying, a repeatedly-broken release will
re-deploy and re-revert once per `sync` run until a fixed release is published. In CLI mode
each run exits red; a "skip a known-failed release" backoff is planned for daemon mode.

## Verification per executor kind

`compose-ps` resolves the compose project it checks at verify time:

- **compose-over-ssh** — the configured `project_dir` / `env_file`.
- **symlink-release** — the active release (`current/.env`).
- **blue-green** — the **idle** slot (the one a deploy just staged), so a compose-ps probe
  gates on the slot about to receive traffic.

`gantry switch` runs the environment's verify probes against the idle slot **before** flipping
the pointer; a failing probe refuses the switch and leaves live traffic untouched.

For blue-green, use `compose-ps` or `command` probes to check the idle slot — they run over SSH
and need no HTTP routing. An `http` probe hits the public URL, which serves the **live** slot;
to http-check the idle slot, use a `command` probe (e.g. `curl` its internal port).

Because a blue-green deploy only stages the idle slot (live is untouched), a failed deploy
verify **holds** — it never auto-rolls-back, and `verify_on_failure: rollback` has no effect on
a blue-green environment. The pre-switch verify gate is the blue-green safety mechanism.
