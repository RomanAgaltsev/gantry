# Demo example

A generic two-component setup (`api` + `web`) that exercises the full gantry slice-1
flow: consume the latest GitLab releases, pin the resolved image references into a
per-environment dotenv file, and deploy with `docker compose pull && up -d` over SSH.

> **This demo is intentionally unrelated to any specific system.** It exists to show
> that gantry is driven entirely by `gantry.yaml` — there is no hard-coded,
> system-specific logic. Point the same binary at a different config and it
> orchestrates a different fleet.

## What's here

- [`gantry.yaml`](gantry.yaml) — two components (`api`, `web`) deployed to a single
  `test` environment over SSH to `app-host`, pinning into `.env.versions.test`.

## Prerequisites

- The `gantry` binary (`task build` from the repo root, or `go build -o gantry ./cmd/gantry`).
- A GitLab token with `read_api` scope on the `demo/api` and `demo/web` projects.
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

## Adapting it

To use this against your own fleet, edit `gantry.yaml`:

- Set `forge.base_url` to your GitLab instance.
- Replace the `components` `project` paths and `pin_key` names.
- Point `connections.app-host` at your host and update the SSH `${file:...}` paths.
- Adjust `executor.project_dir` and `compose_files` to match the host layout.

See [../../docs/configuration.md](../../docs/configuration.md) for the full field
reference.
