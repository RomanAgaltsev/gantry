# Demo example

A generic setup that exercises the full gantry flow: consume the latest GitLab
releases of the first-party components, pin the resolved image references into a
per-environment dotenv file, and deploy with `docker compose pull && up -d` over SSH.
It also includes an explicit-pin component to show how a third-party image
(`postgres`) lives alongside forge-tracked ones.

> **This demo is intentionally unrelated to any specific system.** It exists to show
> that gantry is driven entirely by `gantry.yaml` — there is no hard-coded,
> system-specific logic. Point the same binary at a different config and it
> orchestrates a different fleet.

## What's here

- [`gantry.yaml`](gantry.yaml) — three components deployed to a single `test`
  environment over SSH to `app-host`, pinning into `.env.versions.test`:
  - `api` and `web` — first-party, forge-tracked (`source: { forge: release }`,
    the default); gantry resolves their images from the latest GitLab Release.
  - `postgres` — explicit-pin (`source: { pin: explicit }`); its image is
    maintained by hand or Renovate in the pin file, never by the poller.

## Prerequisites

- The `gantry` binary (`task build` from the repo root, or `go build -o gantry ./cmd/gantry`).
- A GitLab token with `read_api` scope on the `demo/api` and `demo/web` projects.
  (`postgres` is explicit-pin, so it needs no forge access.)
- The directory you run from is a git working tree (gantry commits the pin file).
- For an actual deploy: SSH access to the host in `connections.app-host`, with the
  private key and `known_hosts` available at the `${file:...}` paths.

## Walkthrough

```bash
# 1. Provide the forge token referenced by ${env:GANTRY_FORGE_TOKEN}
export GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx

# 2. See the pending pin changes (read-only — no commit, no deploy)
gantry plan --env test --config examples/demo/gantry.yaml

# 3. Apply them: pin + commit-on-diff + deploy over SSH
gantry sync --env test --config examples/demo/gantry.yaml

# 4. See the recorded ok deploy
gantry history --env test --config examples/demo/gantry.yaml

# 5. Compare current pins against the latest available releases
gantry status --env test --config examples/demo/gantry.yaml

# 6. Snapshot the green test set into prod
gantry promote --from test --to prod --config examples/demo/gantry.yaml

# 7. Revert prod to its previous set
gantry rollback --env prod --config examples/demo/gantry.yaml
```

`plan` prints lines like `API_IMAGE: reg/api:v1.3.0 -> reg/api:v1.4.0`, or
`up to date; no changes` when the pins already match the latest releases. `sync` is a
no-op when nothing changed — it commits and deploys only on a real diff.

### See everything at a glance

`gantry status --all` prints the cross-environment matrix — which version is
pinned where, what the latest release is (with a `!` on anything that lags),
and each environment's last deploy health. Add `--log-format json` to any
command to get structured logs on stderr.


## The explicit-pin component

`POSTGRES_IMAGE` is maintained directly in `.env.versions.test`, not derived from a
forge Release. Set it by hand (or let Renovate bump it):

```dotenv
POSTGRES_IMAGE=postgres:16.4
```

Because it is declared `source: { pin: explicit }`:

- `gantry sync` leaves it alone — the poller never reads a registry for it and never
  overwrites it (single-writer rule).
- `gantry status` shows it as `latest=(untracked)`, since there is no forge release to
  compare against.

When the explicit pin changes (a Renovate or manual bump committed to the pin file),
reconcile the running stack to the whole current pin file — every component, both
sources — with `deploy`:

```bash
# Apply the committed pin file to the host (used after a Renovate/explicit bump)
gantry deploy --env test --config examples/demo/gantry.yaml
```

Unlike `sync`, `deploy` does not consult the forge or write the pin file; it just
deploys what is already committed.

## Secrets beyond env/file

The demo resolves its forge token from `${env:GANTRY_FORGE_TOKEN}` and its SSH key/known_hosts
from `${file:…}` paths. Those two schemes are built in; gantry also supports `${cmd:…}`
(shell out to a tool like `op`/`pass`), `${sops:file#key}` (Mozilla SOPS), and
`${vault:path#field}` (HashiCorp Vault) — useful when credentials live in a secret store
rather than a plain env var or file. For example, a registry password from SOPS:

```yaml
registries:
  registry.example.com:
    user: ${cmd:op read op://vault/reg/user}
    password: ${sops:secrets.enc.yaml#reg.password}
```

These shell out to the `cmd`/`sops`/`vault` binaries, which must be installed on the host
(the default distroless image ships only `env`/`file`/`cmd`). See
[../../docs/secrets.md](../../docs/secrets.md) for the full scheme reference.

## Verifying deploys

Both environments carry a `verify:` block, so after a deploy gantry runs health probes
before recording the outcome as healthy. `test` uses a single `compose-ps` check (every
compose service on the host is running, and healthy if it declares a healthcheck); `prod`
adds an HTTP probe against `https://app.example.com/healthz` (run from gantry). A failed
probe records `result: failed, healthy: false` and exits non-zero — the stack is left as
deployed, not rolled back.

The top-level `promote.require_healthy: true` then tightens the promotion gate: a `test` set
is promoted to `prod` only once its `test` deploy is recorded `ok` **and** `healthy: true`
(which is why `test` is verified too, not just `prod`). A green deploy that has not verified
healthy is refused. See [../../docs/verification.md](../../docs/verification.md).

## Detecting drift

The `drift:` block sets how long a published-but-unpinned release may sit before gantry
calls it out. With `threshold: 7d`, once a component's latest GitLab Release has been
available for more than seven days without its pin being updated, `gantry drift` reports
it (the threshold also accepts `h`/`m` units, e.g. `72h`):

```bash
# Check one environment (read-only — no commit, no deploy)
gantry drift --env test --config examples/demo/gantry.yaml

# Check every track-mode environment — meant to run in CI
gantry drift --all --config examples/demo/gantry.yaml
```

`drift` exits `0` when every pin is current and `3` when any tracked component has
drifted, so a scheduled `gantry drift --all` turns an un-consumed release into a red
build. Only track-mode environments are scanned, and explicit-pin components
(`postgres`) are skipped — gantry has no notion of their "latest". See
See [../../docs/drift.md](../../docs/drift.md) for the full model.

## Run it continuously

`gantry serve` runs the reconcile loop as a long-lived process — it runs `sync`
on an interval for you, under a single-writer lock, so a track-mode environment
stays pinned to the latest releases without a CI schedule:

```bash
# Reconcile `test` every 60s (the default interval) until interrupted
gantry serve --config examples/demo/gantry.yaml

# Reconcile faster while developing
gantry serve --interval 15s --config examples/demo/gantry.yaml
```

`/healthz` is served on `:9713`; stop it with `Ctrl-C` or `SIGTERM`. While the
daemon runs, the mutating verbs (`sync`, `deploy`, `promote`, …) refuse to act.
See [../../docs/daemon.md](../../docs/daemon.md).

## Adapting it

To use this against your own fleet, edit `gantry.yaml`:

- Set `forge.base_url` to your GitLab instance.
- Replace the `components` `project` paths and `pin_key` names.
- Point `connections.app-host` at your host and update the SSH `${file:...}` paths.
- Adjust `executor.project_dir` and `compose_files` to match the host layout.

See [../../docs/configuration.md](../../docs/configuration.md) for the full field
reference.
