# gantry

**gantry** is a non-Kubernetes release orchestrator. It reads the latest published
release of each of your components from a forge (GitLab today), writes the resolved
immutable image references into a per-environment dotenv pin file (committing only
when something actually changed), and reconciles the running environment with
`docker compose pull && up -d` over SSH.

## The gap it fills

Plenty of teams run their services with Docker Compose on plain hosts rather than
Kubernetes. They still want the things Kubernetes-style GitOps gives you: a single
declarative description of which version runs where, an auditable history of every
version bump, and a one-command path from "a new release exists" to "it is running
in the test environment." gantry provides exactly that for the Compose-over-SSH
world — config-driven, with no system-specific logic baked in.

## Current capabilities

- **Consume** the latest GitLab Release of each component and parse its embedded
  release-metadata block (`repository:tag`, digest, commit, changelog). When the
  metadata carries a digest, the pin is written as `repository:tag@sha256:…` so the
  pulled image cannot drift if the tag is later re-pushed.
- **Pin** the resolved image references into a per-environment dotenv file and
  commit the change to git — but only when the pin actually differs (commit-on-diff;
  re-runs are no-ops).
- **Deploy** the environment over SSH: write the env file on the host, `docker login`
  any referenced private registries, then `docker compose pull` and `up -d`.
- **Inspect** before acting: `plan` shows pending pin changes without writing, and
  `status` shows current pins vs. the latest available releases.

Everything I/O — GitLab HTTP, SSH, git commits — sits behind interfaces, so the core
logic is tested without live infrastructure. gantry never shells out at runtime: it
uses `go-git` and `golang.org/x/crypto/ssh` directly.

## Quickstart

```bash
# Build the binary
task build            # or: go build -o gantry ./cmd/gantry

export GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx

# See what would change, then apply it
./gantry plan   --env test --config gantry.yaml
./gantry sync   --env test --config gantry.yaml

# Compare current pins against the latest releases
./gantry status --env test --config gantry.yaml
```

A complete, runnable configuration lives in [`examples/demo`](examples/demo/) — a
generic two-component setup that is intentionally unrelated to any specific system,
to show that gantry is driven entirely by `gantry.yaml`.

See [docs/getting-started.md](docs/getting-started.md) for a full walkthrough and
[docs/configuration.md](docs/configuration.md) for the complete config reference.

## Roadmap

Current version covers consume → pin → deploy for a single track-mode
environment. Planned next:

- **Promotion** — execute the `promote_from` env model (copy pins from an upstream
  environment instead of tracking the forge directly).
- **Rollback** and drift detection.
- **More forges** — GitHub releases adapter alongside GitLab.
- **More executors** — symlink/blue-green deploy strategies.
- **Health verification** after deploy, notifications, and a status matrix across
  environments.

## License

Released under the [MIT License](LICENSE).
