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

# 4. Compare current pins against the latest available releases
gantry status --env test --config examples/demo/gantry.yaml
```

`plan` prints lines like `API_IMAGE: reg/api:v1.3.0 -> reg/api:v1.4.0`, or
`up to date; no changes` when the pins already match the latest releases. `sync` is a
no-op when nothing changed — it commits and deploys only on a real diff.

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

## Adapting it

To use this against your own fleet, edit `gantry.yaml`:

- Set `forge.base_url` to your GitLab instance.
- Replace the `components` `project` paths and `pin_key` names.
- Point `connections.app-host` at your host and update the SSH `${file:...}` paths.
- Adjust `executor.project_dir` and `compose_files` to match the host layout.

See [../../docs/configuration.md](../../docs/configuration.md) for the full field
reference.
