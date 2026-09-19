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

## Run it locally

Everything below describes a real fleet. **[`stand/`](stand/) makes this config true
locally** — a fake GitLab, a Docker-in-Docker host reachable over SSH, and the published
gantry image, wired so `gitlab.example.com`, `192.0.2.10` and the `/run/secrets/…` paths in
`gantry.yaml` all resolve. The config is run **unmodified**: it is the artifact under proof,
and `git diff --exit-code examples/demo/gantry.yaml` is clean after a full demo run.

```bash
task demo:up
```

That generates an SSH keypair, a demo CA and a `gitlab.example.com` certificate,
`known_hosts`, and a scratch git tree for pin commits; then builds and starts the stand.
Everything generated is gitignored, so `git status` stays clean.

From then on every `gantry` command in this document runs as:

```bash
task demo:gantry -- <the same arguments>
```

with **no `--config`**. The shipped `gantry.yaml` is mounted into the container's working
directory and found by gantry's default config path. That is deliberate: gantry roots the
pin store and the ledger at the *config file's* directory, not at the working directory, so
pointing `--config` elsewhere would put pin files next to your checkout.

Other targets: `task demo:down`, `task demo:reset` (discards published releases),
`task demo:logs`, `task demo:host` (a shell on the deploy target), and
`task demo:release -- <component> <version> [image_repository] [image_tag] [built_at]` to
publish into the fake forge.

**Requires** Docker and a POSIX shell (Git Bash, WSL, or any Unix). Nothing here runs in
`task ci`: the stand asserts nothing and is driven by hand.

> **Slice 1 is the `test` environment only.** `gantry.yaml` also declares `prod`
> (symlink-release) and `front` (blue-green); neither host layout is built by the stand, so
> `promote` and `rollback` are out of scope — see steps 6 and 7 of the walkthrough.

### The six scenarios

Each was run against the stand. The output below is what it printed.

**1. Cold start** — `plan`, then `sync`.

```
$ task demo:gantry -- plan --env test
API_IMAGE:  -> traefik/whoami:v1.10.2
WEB_IMAGE:  -> nginx:1.25
missing pins (in config, not in pin file): API_IMAGE, WEB_IMAGE

$ task demo:gantry -- sync --env test
msg="pin written" env=test commit=32d596e changes=2
msg="deploy recorded" env=test by=sync result=ok commit=32d596e
deployed
```

Three containers are then up on the target, and the pin file is committed in the scratch
tree (`32d596e chore(test): pin 2 component(s)`, followed by a ledger commit).

`POSTGRES_IMAGE` is seeded by `setup.sh` rather than resolved. It is explicit-pin, so
gantry never writes it — and without a value the first deploy fails with *"service postgres
has neither an image nor a build context specified"*.

**2. A new release** — publish, then watch it land.

```
$ task demo:release -- api 1.5.0 traefik/whoami v1.11.0
$ task demo:gantry -- status --env test
API_IMAGE            pinned=traefik/whoami:v1.10.2   latest=traefik/whoami:v1.11.0
WEB_IMAGE            pinned=nginx:1.25               latest=nginx:1.25
POSTGRES_IMAGE       pinned=postgres:16.4            latest=(untracked)

$ task demo:gantry -- plan --env test
API_IMAGE: traefik/whoami:v1.10.2 -> traefik/whoami:v1.11.0

$ task demo:gantry -- sync --env test
deployed
```

**3. Drift** — a release published more than `drift.threshold` (7d) ago and not deployed.

```
$ task demo:release -- api 1.6.0 traefik/whoami v1.12.0 2026-09-09T10:00:00Z
$ task demo:gantry -- drift --all
DRIFT test/api: pinned traefik/whoami:v1.11.0, latest 1.6.0 published 10d ago (>7d)
DRIFT front/api: pinned (unpinned), latest 1.6.0 published 10d ago (>7d)
```

Exit code **3**, which is what makes it usable as a CI gate.

**4. Failed verification → rollback** — publish an image that cannot stay up.

```
$ task demo:release -- api 1.7.0 alpine 3
$ task demo:gantry -- deploy --env test
msg="deploy recorded" env=test by=deploy result=failed
msg="deploy recorded" env=test by=auto-rollback result=ok
verify failed for test; rolled back to 7938b64
gantry: verify failed, rolled back: verify "test": service api is "restarting", not running
```

The pin returns to `traefik/whoami:v1.11.0` and the container comes back healthy.

> **Use `deploy`, not `sync`, to see this.** The deployed services carry
> `restart: unless-stopped`, so a container that exits is restarted rather than left dead.
> `sync` runs `compose-ps` within seconds of `up -d` and can observe the service while it is
> still `running`, reporting success. Once the container is visibly looping, `deploy`
> re-verifies and fails reliably.

**5. The explicit pin** — operator-managed, and the poller leaves it alone.

```
$ vi examples/demo/stand/.work/.env.versions.test   # POSTGRES_IMAGE=postgres:16.3
$ git -C examples/demo/stand/.work commit -am "ops: pin postgres to 16.3"
$ task demo:gantry -- deploy --env test
deployed 3 pin(s) to test        # postgres is now 16.3

$ task demo:gantry -- sync --env test
up to date; no changes           # POSTGRES_IMAGE untouched
```

**The edit must be committed.** The pin store is git-backed, so an uncommitted change to the
pin file is invisible to gantry — `deploy` will report success having deployed the
*committed* value.

**6. The daemon** — reconciles unattended and holds a lock.

```
$ task demo:gantry -- serve --interval 15s      # in one terminal
$ task demo:release -- api 1.9.0 traefik/whoami v1.10.1
  msg="pin written" env=test commit=0dfb9dd changes=1
  msg="deploy recorded" env=test by=sync result=ok commit=0dfb9dd

$ task demo:gantry -- sync --env test           # in another
gantry: a gantry daemon is reconciling this repo (.gantry/serve.lock); stop it or wait
```

> **`serve` has no `--env` flag** — it reconciles *every* environment in the config. In
> slice 1 that means it also tries `front` each interval and logs
> `reconcile failed env=front error="deploy \"front\": write blue env: …"`. That noise is
> expected here; it is the blue-green host layout being absent, not a gantry fault.

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
#    NOT AVAILABLE ON THE LOCAL STAND: `prod` is a symlink-release environment on
#    prod-host, and the stand builds the `test` target only. Run this against a real
#    fleet, or expect it to fail resolving connections.prod-host.
gantry promote --from test --to prod --config examples/demo/gantry.yaml

# 7. Revert prod to its previous set
#    NOT AVAILABLE ON THE LOCAL STAND, for the same reason as step 6.
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
