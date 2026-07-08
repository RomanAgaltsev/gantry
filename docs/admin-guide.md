# gantry Administrator & Operator Guide

A single, comprehensive, step-by-step guide to running **gantry** — from a fresh
download through the most advanced continuous-deployment topologies. It is written to
be read start-to-finish once, then used as a reference. It is self-contained: you do
not need any other document to operate gantry, though the focused reference pages
(linked at the end) go deeper on individual subsystems.

> **What gantry is, in one sentence.** gantry is a non-Kubernetes release orchestrator:
> it reads the latest published release of each of your components from a forge (GitLab
> or GitHub), pins the resolved immutable image references into per-environment dotenv
> files committed to git, deploys them over SSH (plain compose, symlink-release, or
> blue-green), verifies health, records every outcome in a git-tracked deploy ledger,
> and gates promotion to production on that ledger.

---

## Table of contents

1. [Mental model — how gantry thinks](#1-mental-model--how-gantry-thinks)
2. [When to use gantry (and when not to)](#2-when-to-use-gantry-and-when-not-to)
3. [Installation](#3-installation)
4. [Prerequisites & host preparation](#4-prerequisites--host-preparation)
5. [Core concepts glossary](#5-core-concepts-glossary)
6. [First run — an end-to-end tutorial](#6-first-run--an-end-to-end-tutorial)
7. [Authoring `gantry.yaml` — the full reference](#7-authoring-gantryyaml--the-full-reference)
8. [Forges & the release-metadata contract](#8-forges--the-release-metadata-contract)
9. [Executors in depth](#9-executors-in-depth)
10. [Verification](#10-verification)
11. [Promotion, rollback & the deploy ledger](#11-promotion-rollback--the-deploy-ledger)
12. [Drift detection](#12-drift-detection)
13. [Secrets](#13-secrets)
14. [Notifications](#14-notifications)
15. [Running as a daemon (`serve`)](#15-running-as-a-daemon-serve)
16. [Observability & metrics](#16-observability--metrics)
17. [CI integration recipes](#17-ci-integration-recipes)
18. [Day-2 operations runbook](#18-day-2-operations-runbook)
19. [Security model & hardening](#19-security-model--hardening)
20. [CLI command reference](#20-cli-command-reference)
21. [Troubleshooting & FAQ](#21-troubleshooting--faq)

---

## 1. Mental model — how gantry thinks

Internalize four ideas and everything else follows.

**1. Git is the only state store.** There is no database, no control-plane service, no
hidden cluster state. gantry's entire world is files in a git working tree:

- **Pin files** — per-environment dotenv files (e.g. `.env.versions.test`) holding the
  resolved image reference for each component. These *are* "which version runs where."
- **The deploy ledger** — an append-only JSONL file at `.gantry/deploys.jsonl` recording
  the outcome of every deploy.
- **`gantry.yaml`** — the operator-authored configuration describing forges, components,
  environments, and how to deploy.

Because state is git, crash-safety, auditability, and "`git log` as the debugging UI"
all fall out for free. A daemon is stateless: each loop just reads git.

**2. The forge is the source of releases, not of truth.** gantry *reads* the latest
release of each component from GitLab or GitHub and *writes* the resolved image reference
into a pin file. It never guesses an image from a tag name — each release must carry an
explicit metadata block (see §8).

**3. The ledger is the promotion gate.** Every deploy records its outcome keyed by
`(environment, pin-commit)`. Promotion to prod reads that ledger and refuses any revision
that does not have a green (optionally *healthy*) deploy on the source environment. "Prod
requires a proven revision" is thus *enforced*, not merely a convention.

**4. Promotion is frozen, rollback is ledger-targeted.** `promote` reads the source pin
file **as committed at the gated SHA**, never "current upstream," closing the race between
validating a set and shipping it. `rollback` targets the last *green* ledger entry, not the
parent commit, so repeated rollbacks walk backward through known-good states instead of
oscillating onto a bad one.

### The data flow, end to end

```
   forge (GitLab/GitHub)                     deploy hosts (SSH)
   ┌───────────────────┐                     ┌──────────────────┐
   │ Release + metadata │                     │ docker compose   │
   └─────────┬─────────┘                     │  pull && up -d   │
             │ LatestRelease()                └────────▲─────────┘
             ▼                                          │
   ┌───────────────────┐   commit    ┌─────────────────┴───────┐
   │ resolve image ref  │──────────▶ │ pin file  .env.versions  │
   │ repo:tag@sha256    │            │ ledger    deploys.jsonl  │  (all in git)
   └───────────────────┘            └─────────────────────────┘
```

---

## 2. When to use gantry (and when not to)

**Use gantry when** you run services with Docker Compose on plain hosts (not Kubernetes)
and you want the GitOps guarantees Kubernetes users take for granted: a single declarative
record of which version runs where, an auditable history of every version bump, a verified
path from "a release exists" to "it is running in test," and a promotion to production that
can only ship a revision a green test deploy has already proven.

**gantry is a good fit when:**

- You deploy container images over SSH to a fixed set of hosts.
- You can publish (or are willing to publish) a machine-readable release per component.
- You want promotion gated on real evidence, not a human's memory of "test looked fine."

**Look elsewhere when:**

- You already run Kubernetes/Nomad — use their native GitOps (Argo CD, Flux) instead.
- You need per-request traffic shaping, autoscaling, or service mesh features. gantry
  orchestrates *releases*, not runtime traffic (blue-green is a coarse pointer flip).
- Your images are chosen by something other than "latest release of a repo" and you do not
  want to model them as explicit pins.

---

## 3. Installation

gantry is a single static Go binary. Pick whichever install path suits you.

### 3.1 Build from source

You need **Go 1.26 or newer**.

```bash
git clone https://github.com/RomanAgaltsev/gantry.git
cd gantry
task build            # produces ./bin/gantry
# or, without Task:
go build -o gantry ./cmd/gantry
```

Verify:

```bash
./bin/gantry version
```

### 3.2 `go install`

```bash
go install github.com/RomanAgaltsev/gantry/cmd/gantry@latest
gantry version
```

### 3.3 Release binaries

Tagged releases publish prebuilt binaries (via GoReleaser) on the GitHub Releases page.
Download the archive for your OS/arch, extract, and place `gantry` on your `PATH`.

### 3.4 Docker image

The repository ships a distroless image build:

```bash
docker build -t gantry:dev --build-arg VERSION=dev .
docker run --rm gantry:dev version
```

Because gantry reads `gantry.yaml`, resolves secrets from the environment or mounted files,
and commits to a git working tree, mount those when you run it:

```bash
docker run --rm \
  -e GANTRY_FORGE_TOKEN \
  -v "$PWD:/work" -w /work \
  -v /run/secrets:/run/secrets:ro \
  gantry:dev sync --env test --config gantry.yaml
```

> The distroless runtime ships only the pure-Go secret backends (`env`, `file`,
> `vault-http`) plus `cmd` when a `cmd` binary is present. To use `sops`/`vault` CLIs from
> a container, use a fatter image or resolve those secrets in CI first — see §13.

### 3.5 Where gantry runs

gantry runs on an **orchestrator machine** that has:

- the git working tree holding `gantry.yaml`, the pin files, and the ledger;
- network reach to your forge's API;
- SSH reach to each deploy host.

That can be a CI runner (one-shot invocations) or a long-lived host running the daemon.
It does **not** run *on* the deploy hosts — it drives them over SSH.

---

## 4. Prerequisites & host preparation

Before your first `sync`, make sure the following are in place.

### 4.1 A forge account and API token

- **GitLab:** a personal/project/group access token with the **`read_api`** scope.
- **GitHub:** a token with read access to the repositories' releases (optional for public
  repos — gantry can call anonymously).

Never place the token literally in `gantry.yaml`; it is referenced as a secret (§13).

### 4.2 The orchestrator git repo

gantry commits the pin file and ledger into a git working tree. Create (or reuse) a repo —
often called the **environment** or **deploy** repo — that will hold:

```
gantry.yaml
.env.versions.test          # pin file, maintained by gantry
.env.versions.prod          # pin file, maintained by gantry
.gantry/deploys.jsonl       # ledger, maintained by gantry
compose.yaml                # (optional) the compose project you ship to hosts
```

gantry **must own the working tree**: it builds each commit from the git index and refuses
to run when unrelated changes are already staged. Keep operator edits and gantry runs from
stepping on each other (commit or stash before running).

### 4.3 The deploy hosts

Each deploy host needs:

- **Docker Engine + the Compose v2 plugin** (`docker compose`, not the legacy
  `docker-compose`).
- **An SSH account** gantry logs in as, whose user can run `docker` (in the `docker` group
  or via sudo configured for non-interactive use).
- **A project directory** (e.g. `/opt/app`) where gantry writes the env file and runs
  compose. For `symlink-release` and `blue-green`, gantry manages subdirectories under it.

### 4.4 SSH host-key pinning (`known_hosts`)

gantry **never** trusts an unknown host key on first use. You must provide the contents of
a `known_hosts` file for every host it connects to. Gather them once:

```bash
ssh-keyscan -H deploy-host.example.com >> known_hosts
```

Then reference that file's contents as a secret in the connection (§7.2).

### 4.5 Registry access on the hosts

If images live in a private registry, gantry logs in to it on the host before pulling. Have
a registry username + token ready to reference as secrets (§7.5). gantry only logs in to
registries actually referenced by the current pin set.

---

## 5. Core concepts glossary

| Term | Meaning |
| --- | --- |
| **Component** | A buildable repo whose image gantry pins. Either *forge-tracked* (image derived from its latest release) or *explicit-pin* (image maintained by hand/Renovate). |
| **Pin / pin file** | A dotenv key=value mapping a component's `pin_key` to its resolved `repository:tag[@sha256:…]`. One pin file per environment. |
| **Environment** | One deploy target — a name, a pin file, a source (track vs. promote), and an executor. |
| **Forge** | The GitLab/GitHub adapter gantry reads releases from. |
| **Release-metadata block** | A machine-readable JSON block embedded in a release description that names the image repository, tag, digest, etc. gantry never guesses images from tag names. |
| **Executor** | *How* gantry reconciles a host to a pin set: `compose-over-ssh`, `symlink-release`, or `blue-green`. |
| **Verification / probe** | A post-deploy health check (`http`, `compose-ps`, `command`). Recorded as the deploy's `healthy` flag. |
| **Ledger** | The append-only `.gantry/deploys.jsonl` recording every deploy's outcome, keyed by `(environment, pin-commit)`. |
| **Promotion** | Copying a *frozen*, green pin set from an upstream environment to a downstream one. |
| **Rollback** | Redeploying the last known-good (green) pin set from the ledger. |
| **Drift** | A published release left unconsumed past a threshold. |
| **Track mode** | An environment whose pins come from the forge (`source: { track: latest }`). Auto-reconciled by the daemon. |
| **Promote mode** | An environment whose pins are copied from an upstream (`source: { promote_from: … }`). Never auto-reconciled. |
| **Secret reference (SecretRef)** | A `${scheme:arg}` string that resolves a credential at use time. Inline literals are rejected. |

---

## 6. First run — an end-to-end tutorial

This walks the complete lifecycle against a two-environment setup. A complete, runnable
version lives in [`examples/demo`](../examples/demo/).

### 6.1 Write a minimal `gantry.yaml`

```yaml
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_FORGE_TOKEN}
connections:
  app-host:
    address: 192.0.2.10
    ssh:
      user: deploy
      key: ${file:/run/secrets/app_ssh_key}
      known_hosts: ${file:/run/secrets/known_hosts}
components:
  - { id: api, project: demo/api, pin_key: API_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor:
      kind: compose-over-ssh
      connection: app-host
      project_dir: /opt/demo
      compose_files: [compose.yaml]
      env_file: .env.versions.test
```

### 6.2 Provide the forge token

```bash
export GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

### 6.3 Validate (read-only)

Check the config end-to-end — schema, that every referenced secret resolves, and that the
pin files agree with the declared components:

```bash
gantry validate --config gantry.yaml
```

On success it prints `config valid`. Unresolved secrets or schema errors fail non-zero;
orphan/missing pins are reported as warnings (they do not fail validation).

### 6.4 Plan (read-only)

See the pin changes a `sync` would make, without writing or deploying:

```bash
gantry plan --env test
# API_IMAGE: reg/api:v1.3.0 -> reg/api:v1.4.0
```

If everything is already pinned to the latest release, it prints `up to date; no changes`.

### 6.5 Sync (pin + deploy)

`sync` resolves the latest releases and, if the pins differ from what is recorded, it:

1. writes the updated pin file and **commits it** (commit-on-diff);
2. writes the env file on the host over SSH;
3. logs in to any private registries the images reference;
4. runs `docker compose pull` then `docker compose up -d`;
5. runs the environment's `verify:` probes and records the outcome in the ledger.

```bash
gantry sync --env test
```

If nothing changed, `sync` is a no-op: no commit, no deploy.

> **Push the commit.** gantry commits **locally**; it does not push by default. For the
> history to survive and for promotion/rollback on another machine to see it, push it
> yourself (in CI, add `git push` after `sync`). A daemon can instead use `git.remote` to
> pull/push automatically — see §15.

### 6.6 Check status

```bash
gantry status --env test
# API_IMAGE   pinned=reg/api:v1.3.0   latest=reg/api:v1.4.0
gantry status --all           # the cross-environment matrix
```

### 6.7 Add prod and promote

Add a second environment sourced *from* test:

```yaml
  - name: prod
    source: { promote_from: test }
    pin_file: .env.versions.prod
    executor:
      kind: compose-over-ssh
      connection: prod-host
      project_dir: /opt/demo
      compose_files: [compose.yaml]
      env_file: .env.versions.prod
```

Once `test` has a green deploy, promote that exact set and roll back if needed:

```bash
gantry promote --from test --to prod --dry-run   # preview + gate decision
gantry promote --from test --to prod             # promote the latest GREEN test set
gantry history  --env prod                        # confirm the outcome
gantry rollback --env prod                        # revert to the last known-good set
```

You have now exercised the full loop: resolve → pin → deploy → verify → record → promote →
roll back.

---

## 7. Authoring `gantry.yaml` — the full reference

gantry is driven entirely by one YAML file (default `gantry.yaml`, override with
`--config`). It is read, defaulted, and validated before any forge call or deploy; a
validation error stops the command early.

### 7.0 Top-level structure

```yaml
forge:        { ... }              # required — which forge to read releases from
connections:  { <name>: { ... } }  # named deploy targets (the inventory)
components:   [ { ... } ]          # the buildable repos whose images are pinned
environments: [ { ... } ]          # deploy-target environments
registries:   { <host>: { ... } }  # optional — private registry credentials
git:          { ... }              # optional — pin-commit identity + remote sync
drift:        { ... }              # optional — drift threshold
promote:      { ... }              # optional — promotion gate strictness
notifications: [ { ... } ]         # optional — event channels
daemon:       { ... }              # optional — serve loop settings
secrets:      { ... }              # optional — Vault defaults
```

### 7.1 `forge`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `kind` | string | yes | `gitlab` or `github`. |
| `base_url` | string | yes* | Base URL, e.g. `https://gitlab.example.com`. GitHub defaults to `https://api.github.com`; GitHub Enterprise uses `https://<host>/api/v3`. |
| `token` | SecretRef | yes** | API token. GitLab needs `read_api`; GitHub read access to releases (omit to call public repos anonymously). |
| `metadata_marker` | string | no | Namespace of the release-metadata block. Defaults to `gantry-release-metadata`. |

```yaml
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_FORGE_TOKEN}
  metadata_marker: gantry-release-metadata
```

### 7.2 `connections`

A map of named deploy targets. Environments reference one by name via `executor.connection`.

```yaml
connections:
  app-host:
    address: 192.0.2.10          # bare host → port 22; "host:2222" honored
    ssh:
      user: deploy
      key: ${file:/run/secrets/app_ssh_key}       # PEM private key (SecretRef)
      known_hosts: ${file:/run/secrets/known_hosts}  # REQUIRED — no trust-on-first-use
```

| Field | Required | Description |
| --- | --- | --- |
| `address` | yes | Host address; `host:port` honored, else default SSH port 22. |
| `ssh.user` | yes* | SSH login user. |
| `ssh.key` | yes* | PEM-encoded SSH private key. |
| `ssh.known_hosts` | yes* | Contents of a `known_hosts` file. gantry rejects unknown host keys. |

\* Required whenever an SSH-based executor connects to this host.

### 7.3 `components`

The list of components whose images are pinned.

```yaml
components:
  - { id: api, project: demo/api, pin_key: API_IMAGE }
  - { id: web, project: demo/web, pin_key: WEB_IMAGE }
  - { id: postgres, pin_key: POSTGRES_IMAGE, source: { pin: explicit } }
```

| Field | Required | Description |
| --- | --- | --- |
| `id` | yes | Human-readable identifier. |
| `project` | conditional | Forge project path (`group/repo`) or numeric ID. **Required** for forge-tracked; **must be absent** for explicit-pin. |
| `pin_key` | yes | The dotenv key the resolved image is written under. Must be unique across components. |
| `source` | no | How the pin is resolved: `{ forge: release }` (default) or `{ pin: explicit }`. |
| `drift_threshold` | no | Per-component drift threshold overriding the global `drift.threshold`. |

**`source` semantics:**

- `{ forge: release }` (default) — image derived from the latest forge release; requires
  `project`; tracked by `sync`/`status`.
- `{ pin: explicit }` — image maintained directly in the pin file (by hand or Renovate);
  must **not** set `project`; `sync` skips it, `status` shows `latest=(untracked)`, but
  `deploy` still includes it.

A duplicate `pin_key`, or setting both `forge` and `pin`, is a validation error.

### 7.4 `environments`

Each environment is one deploy target.

```yaml
environments:
  - name: test
    source: { track: latest }        # or: { promote_from: <upstream env> }
    pin_file: .env.versions.test
    executor: { ... }                # see §9
    verify:  [ ... ]                 # optional — see §10
    verify_on_failure: hold          # or: rollback (see §10)
```

| Field | Required | Description |
| --- | --- | --- |
| `name` | yes | Used by `--env`. |
| `source` | yes | `{ track: <ref> }` (track the forge) **or** `{ promote_from: <env> }` (copy from upstream). Exactly one. |
| `pin_file` | yes | Path (relative to the git tree) of the dotenv pin file gantry maintains. |
| `executor` | yes | Deploy backend — see §9. |
| `verify` | no | Post-deploy probes — see §10. |
| `verify_on_failure` | no | `hold` (default) or `rollback`. |

A `promote_from` environment is **not** filled by `sync`; its pins arrive via
`gantry promote --from <upstream> --to <this env>`.

### 7.5 `registries`

Optional. Keyed by **registry host**, credentials used to `docker login` on the host before
pulling. gantry inspects the pinned references and logs in only to the registries actually
used.

```yaml
registries:
  gitlab.example.com:5050:
    user: ${env:GANTRY_REGISTRY_USER}
    password: ${env:GANTRY_REGISTRY_TOKEN}   # fed to docker login --password-stdin
```

The registry host is derived from an image reference by Docker's rule: the first path
segment is the host if it contains `.` or `:` or equals `localhost`; otherwise the image
lives on `docker.io`.

### 7.6 `git`

Optional. Identity for the pin commits gantry makes, plus optional remote sync for the
daemon.

```yaml
git:
  author_name: gantry-bot         # default: gantry
  author_email: gantry@example.com # default: gantry@local
  remote:                          # optional — fleet-safe daemon (see §15)
    name: origin
    branch: main
    pull: true
    push: true
    username: gantry
    token: ${env:GANTRY_GIT_TOKEN} # required when pull/push is on (HTTPS); SSH uses the agent
```

Without `git.remote`, gantry commits **locally** and does not push.

### 7.7 `drift`, `promote`

```yaml
drift:
  threshold: 7d          # default 7d; accepts d/h/m (e.g. 72h, 14d)
promote:
  require_healthy: true  # default false — require a passing verify to promote (§10/§11)
```

`secrets`, `notifications`, and `daemon` are covered in §13, §14, and §15.

### 7.8 SecretRef — the credential shape

Every credential field is a **SecretRef**, never an inline literal:

| Scheme | Resolves to |
| --- | --- |
| `${env:NAME}` | environment variable `NAME` |
| `${file:/path}` | file contents, trimmed |
| `${cmd:prog a b}` | command stdout, trimmed |
| `${sops:file#key}` | a key from a SOPS-decrypted file |
| `${vault:path#field}` | a field from a Vault KV secret (CLI) |
| `${vault-http:path#field}` | a field from Vault KV v2 over native HTTP (no binary) |

A non-`${...}` value is an error — gantry refuses to read inline secrets from the config.
See §13 for the full backend reference.

---

## 8. Forges & the release-metadata contract

This is the single most important thing to get right when adopting gantry.

### 8.1 gantry does not guess images from tags

For each forge-tracked component, gantry resolves "latest release" and reads an explicit
metadata block from the release description. **A release with a missing or invalid metadata
block is a hard error** — gantry never silently skips a release.

### 8.2 The metadata block

Embed this in each release's description, delimited by the configured marker:

```
<!-- gantry-release-metadata:v1:start -->
```json
{
  "schema_version": "1",
  "component": "api",
  "semver_version": "v1.4.0",
  "image_repository": "reg/api",
  "image_tag": "v1.4.0",
  "image_digest": "sha256:...",
  "commit_sha": "deadbeef",
  "built_at": "2026-06-18T10:00:00Z",
  "changelog_section": "### Added\n- ..."
}
```
<!-- gantry-release-metadata:v1:end -->
```

- `image_repository` and `image_tag` are **required**; `built_at` must be RFC 3339.
- When `image_digest` is present, gantry pins `repository:tag@sha256:…` so a re-pushed tag
  cannot drift the pulled image. Without a digest it falls back to a mutable `repository:tag`.

### 8.3 Latest vs. prerelease

Both forges resolve "latest" to the newest **non-prerelease** release. gantry treats any
`semver_version` with a SemVer prerelease segment (a `-` before any `+` build metadata, e.g.
`v1.3.0-rc1`) as a prerelease and skips it. An empty `semver_version` is treated as stable
so gantry never skips a release merely lacking the field. This keeps GitLab and GitHub
aligned so tagging an RC does not auto-deploy on one forge but not the other.

### 8.4 GitLab specifics

```yaml
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_FORGE_TOKEN}   # read_api scope
components:
  - { id: api, project: group/api, pin_key: API_IMAGE }  # path or numeric project id
```

### 8.5 GitHub specifics

```yaml
forge:
  kind: github
  # base_url defaults to https://api.github.com; Enterprise → https://<host>/api/v3
  token: ${env:GANTRY_FORGE_TOKEN}   # optional for public repos
components:
  - { id: api, project: octo/api, pin_key: API_IMAGE }   # project is owner/repo
```

### 8.6 Emitting the block from your build

The practical adoption step: have each component's CI **publish a release** with the metadata
block whenever it builds an image. A minimal GitLab CI job:

```yaml
publish-release:
  stage: release
  image: registry.gitlab.com/gitlab-org/release-cli:latest
  rules:
    - if: '$CI_COMMIT_TAG'          # run on tags
  script:
    - |
      DIGEST=$(skopeo inspect --format '{{.Digest}}' docker://$CI_REGISTRY_IMAGE:$CI_COMMIT_TAG)
      cat > notes.md <<EOF
      <!-- gantry-release-metadata:v1:start -->
      \`\`\`json
      {"schema_version":"1","component":"api","semver_version":"$CI_COMMIT_TAG",
       "image_repository":"$CI_REGISTRY_IMAGE","image_tag":"$CI_COMMIT_TAG",
       "image_digest":"$DIGEST","commit_sha":"$CI_COMMIT_SHA","built_at":"$(date -u +%Y-%m-%dT%H:%M:%SZ)"}
      \`\`\`
      <!-- gantry-release-metadata:v1:end -->
      EOF
  release:
    tag_name: '$CI_COMMIT_TAG'
    description: notes.md
```

If you cannot yet publish releases and only push mutable registry tags, model those
components as **explicit pins** and bump them by hand or with Renovate until the release
pipeline exists.

---

## 9. Executors in depth

An executor is *how* gantry reconciles a host to a pin set. The kind is per environment; all
kinds run over the same SSH connection.

### 9.1 `compose-over-ssh`

The minimal primitive: write the env file in place and run `docker compose pull && up -d`.
Use it when you do not need versioned release dirs or instant rollback.

```yaml
executor:
  kind: compose-over-ssh
  connection: app-host
  project_dir: /opt/app
  compose_files: [compose.yaml]
  env_file: .env.versions.prod
```

gantry writes the rendered pin set to `project_dir/env_file`, then runs, scoped to the
project dir and compose files:

```
docker compose -f <each compose file> --env-file <env_file> pull
docker compose -f <each compose file> --env-file <env_file> up -d
```

### 9.2 `symlink-release`

Each deploy lands in a **new versioned directory** named by the pin commit, and an atomic
`current` symlink is flipped to it:

```
/opt/app/
  releases/
    abc1234/  .env  .version    <- a past release
    def5678/  .env  .version    <- the new release
  current -> releases/def5678   <- atomically flipped (mv -T rename)
```

- **Atomic config swap** — `current` flips with a single rename; no half-written window.
- **Instant rollback** — old images are already on the host, so a rollback's flip-and-`up`
  is fast (see `rollback --fast`, §11).

```yaml
executor:
  kind: symlink-release
  connection: app-host
  project_dir: /opt/app
  compose_files: [compose.yaml]
  keep: 10        # retain newest 10 release dirs; 0 (default) keeps all
```

`symlink-release` needs no `env_file` (it always uses `current/.env`). The release directory
name is the pin commit SHA, so `gantry history` and the `releases/` dirs line up one-to-one.
`keep: N` prunes older release dirs after a successful deploy (best-effort; the active
release is never pruned).

### 9.3 `blue-green`

Two slots of the same service behind a switchable pointer (typically an nginx upstream).
gantry stages a new version on the **idle** slot, leaves the **live** slot serving, and
promotes by flipping the pointer — instant and instantly reversible.

```yaml
executor:
  kind: blue-green
  connection: front-host
  slots:
    blue:  { project_dir: /opt/front-blue,  compose_files: [compose.yaml] }
    green: { project_dir: /opt/front-green, compose_files: [compose.yaml] }
  pointer:
    link:   /etc/nginx/conf.d/front-upstream.conf   # gantry flips this symlink
    blue:   /etc/nginx/conf.d/upstream-blue.conf     # you provide these two
    green:  /etc/nginx/conf.d/upstream-green.conf
    reload: "nginx -s reload"
```

Model and workflow:

- One environment, one pin file, two slots.
- `gantry sync --env front` (or `deploy`) deploys the pins to the **idle** slot. Live is
  untouched.
- `gantry switch --env front` flips the pointer to the freshly-staged slot, gated on its
  deploy being `ok` and passing the environment's verify probes against the idle slot.
- `gantry rollback --env front` flips the pointer back to the other slot (still on the prior
  version).

gantry never templates nginx — you supply the two upstream confs; gantry only decides which
is active. On a fresh host the first `sync` stages `blue` and the first `switch` creates the
link pointing at it.

```bash
gantry sync   --env front   # stage on idle slot
gantry switch --env front   # promote (flip pointer), gated on ok deploy + idle verify
gantry rollback --env front # flip back
gantry history  --env front # sync/switch/rollback all recorded
```

---

## 10. Verification

After a successful deploy, gantry can run **verification probes** to confirm the environment
is actually healthy — not merely that `docker compose up` exited 0. The result is recorded in
the ledger's `healthy` field, and the promotion gate can require it.

### 10.1 Probes

Configure one or more per environment under `verify:`. **All must pass.**

```yaml
verify:
  - { kind: http, url: https://app.example.com/healthz, expect_status: 200 }
  - { kind: compose-ps }
  - { kind: command, command: "test -f /opt/app/.ready" }
```

| kind | runs from | passes when |
| --- | --- | --- |
| `http` | gantry | `GET url` returns `expect_status` (default 200) |
| `compose-ps` | the host | every compose service is running (and healthy if it declares a healthcheck) |
| `command` | the host | the command exits 0 |

`compose-ps` resolves the project it checks at verify time: the configured dir for
compose-over-ssh, `current/.env` for symlink-release, the **idle** slot for blue-green.

### 10.2 What a failed probe does

A failed probe records `result: failed, healthy: false`, prints the error, and exits
non-zero. By default the stack is **left as deployed** (`verify_on_failure: hold`). To
recover: rerun `sync` in track mode, or fix and `deploy`/`rollback` a promote target. A
failed verification makes the revision un-promotable when the gate requires health.

### 10.3 Auto-rollback on failed verify

Set `verify_on_failure: rollback` to automatically revert to the last known-good pin set when
a verify fails:

```yaml
verify:
  - { kind: compose-ps }
verify_on_failure: rollback
```

- Applies to `sync`, `deploy`, and `promote` (a failed promote reverts the *target*).
  `rollback` and `switch` never auto-roll-back.
- The revert reuses `gantry rollback`; its ledger entry is stamped `by=auto-rollback`.
- The command still exits non-zero — auto-rollback restores service, it does not hide the
  failure.
- Requires at least one `verify` probe.
- **Blue-green never auto-rolls-back** — a deploy only stages the idle slot, so a failed
  deploy verify *holds*; the pre-switch verify gate is its safety mechanism.

### 10.4 Requiring health to promote

```yaml
promote:
  require_healthy: true   # default false
```

With `true`, `gantry promote` accepts only a source revision whose ledger entry is `ok`
**and** `healthy: true`, and "latest green" becomes "latest healthy."

---

## 11. Promotion, rollback & the deploy ledger

### 11.1 The ledger

gantry records every deploy in a git-tracked, append-only ledger at `.gantry/deploys.jsonl`,
keyed by `(environment, pin-file commit SHA)`. Each `sync`, `deploy`, `promote`, and
`rollback` appends one JSON line:

```json
{"environment":"test","pin_commit":"<sha>","result":"ok","healthy":"unknown","image_set":{"SVC_IMAGE":"reg/svc:v2"},"deployed_at":"2026-06-22T10:00:00Z","by":"sync"}
```

`result` is `ok` or `failed`. `healthy` is `unknown` unless verification ran. View it:

```bash
gantry history --env test
```

### 11.2 Promote (test → prod)

`promote` copies the source pin file **as of a specific commit** into the target pin file,
commits, and deploys the target:

```bash
gantry promote --from test --to prod             # promote the latest GREEN test deploy
gantry promote --from test --to prod --sha <c>   # promote a specific commit (must be green)
gantry promote --from test --to prod --dry-run   # preview + gate decision
gantry promote --from test --to prod --only SVC_IMAGE  # promote a subset (warns)
```

Key properties:

- **Frozen snapshot** — gantry promotes the file *as committed at the chosen SHA*, never
  "current upstream," so a poller advancing `test` after your decision cannot leak an
  unvalidated set into `prod`.
- **Gated** — gantry refuses a SHA whose source deploy is missing from the ledger or was not
  `ok` (and, with `require_healthy`, not healthy).
- **Wholesale** — the full set that was green together moves in one commit. `--only`
  advances a subset (never validated together) and prints a warning.
- **DAG advisory** — promoting along an edge other than the target's configured
  `promote_from` prints a warning but proceeds.

### 11.3 Diff two environments

```bash
gantry diff --env test --to prod            # only differing pins (absent renders as -)
gantry diff --env test --to prod --output json
```

### 11.4 Rollback

`rollback` restores an environment to its **last known-good** set — the most recent `ok`
ledger entry older than the current pin commit — commits it, and redeploys:

```bash
gantry rollback --env prod
gantry rollback --env prod --dry-run
gantry rollback --env prod --fast    # symlink-release only: flip current, no pull
```

Because the target comes from the ledger (not the literal parent commit), rollback never
redeploys a `failed` set, and running it again walks further back through good states rather
than oscillating onto a bad one. It refuses when there is no earlier green deploy on record.
Immutable image tags keep prior images pullable, so rollback is just a forward deploy of an
earlier, already-green set. `--fast` works only on symlink-release environments (others error
rather than silently doing a slow redeploy).

### 11.5 Self-healing sync

If a deploy fails *after* its pin commit, the pins are committed but not running. The next
`gantry sync` notices the latest pin commit has no green ledger entry and redeploys it
automatically (reported as `recovered`) — no manual `gantry deploy` needed. See §18 for
the manual recovery path when you want it.

### 11.6 Retiring a component (`prune`)

`plan` reports **orphan pins** — keys present in the pin file but no longer declared in
`gantry.yaml`. Remove the component from `gantry.yaml`, then:

```bash
gantry prune --env test --dry-run   # list orphan keys it would remove
gantry prune --env test             # remove them, commit, redeploy the reduced set
```

`prune` refuses to reduce a set to empty (that would deploy with no images).

---

## 12. Drift detection

**Drift** is a published release left unconsumed too long. For each **track-mode**
environment and each forge-pinned component, a component **has drifted** when its latest
release's image reference differs from the current pin **and** that release was published
more than `drift.threshold` ago (measured from the release's `built_at`). Explicit pins and
promote-target environments are never checked.

```yaml
drift:
  threshold: 7d   # default 7d; per-component override via components[].drift_threshold
```

```bash
gantry drift --env test   # one environment
gantry drift --all        # every track-mode environment (CI gate)
# DRIFT test/api: pinned reg/api:v1.4.2, latest v1.5.0 published 9d ago (>7d)
```

**Exit codes:** `0` no drift · `3` drift detected (gate CI on this) · `1` operational error
· `2` usage error. A newer release still within the threshold is *not* drift.

---

## 13. Secrets

Every credential in `gantry.yaml` is a **SecretRef** resolved at use time. A missing secret
is **always an error** — gantry never substitutes an empty string for a missing env var,
file, or vault field.

### 13.1 Schemes

| scheme | form | reads |
| --- | --- | --- |
| `env` | `${env:NAME}` | env variable (unset = error; set-but-empty = `""`) |
| `file` | `${file:/path}` | file contents, trimmed |
| `cmd` | `${cmd:prog a b}` | command stdout, trimmed |
| `sops` | `${sops:file.enc.yaml#db.password}` | a dotted key from a SOPS-decrypted file |
| `vault` | `${vault:secret/gantry#field}` | a Vault KV field (via the `vault` CLI) |
| `vault-http` | `${vault-http:secret/data/gantry#field}` | a Vault KV v2 field over native HTTP (no binary) |

```yaml
forge:      { token: ${env:GANTRY_FORGE_TOKEN} }
connections:
  host:
    ssh:
      key: ${file:/run/secrets/ssh_key}
      known_hosts: ${file:/run/secrets/known_hosts}
registries:
  registry.example.com:
    user: ${cmd:op read op://vault/reg/user}
    password: ${sops:secrets.enc.yaml#reg.password}
```

### 13.2 Vault defaults

`${vault:…}` and `${vault-http:…}` take their address/token from the optional `secrets.vault`
block (each itself a SecretRef); both default to `${env:VAULT_ADDR}` / `${env:VAULT_TOKEN}`:

```yaml
secrets:
  vault:
    address: ${env:VAULT_ADDR}
    token:   ${env:VAULT_TOKEN}   # or ${file:/run/secrets/vault_token}
```

### 13.3 Binary-dependency caveat

`env`, `file`, and `vault-http` are pure Go and work everywhere (including distroless).
`${cmd:…}`, `${sops:…}`, and `${vault:…}` shell out to host binaries gantry does not vendor.
For a minimal image, prefer `vault-http`, or resolve those secrets in CI before invoking
gantry. A resolver call is bounded to 30s so a hung tool cannot wedge a command or reconcile.

---

## 14. Notifications

gantry pushes events to one or more channels. Notifications are **best-effort**: a failed
send is logged and never fails a deploy, promote, rollback, or drift check.

**Events:** `deployed` · `promoted` · `rolled_back` · `verify_failed` · `drift_alarm` ·
`reconcile_failed` (daemon-only; emitted when a reconcile fails for a non-verify reason).
Each channel may subscribe to a subset with `events:`; omit it to receive all.

```yaml
notifications:
  - kind: webhook
    url: ${env:GANTRY_WEBHOOK_URL}          # generic JSON sink
    events: [deployed, promoted, rolled_back, verify_failed, drift_alarm]
  - kind: slack
    url: ${env:GANTRY_SLACK_WEBHOOK_URL}
  - kind: telegram
    url: https://api.telegram.org/bot<token>/sendMessage
    chat_id: ${env:GANTRY_TELEGRAM_CHAT_ID}  # required for telegram
  - kind: email
    smtp: { host: smtp.example.com, port: 587, username: ops, password: ${file:/run/secrets/smtp}, tls: starttls }
    from: gantry@example.com
    to: [ops@example.com]
    events: [verify_failed, drift_alarm]
```

`webhook`/`slack`/`telegram` are thin wrappers over one webhook core (they differ only in
payload shape). All three require `url`; `telegram` also requires `chat_id`. For `email`,
`smtp.tls` is `starttls` (default) or `implicit` (TLS-on-connect, port 465).

---

## 15. Running as a daemon (`serve`)

`gantry serve` runs the reconcile loop as a long-lived process: it runs the same `sync`
logic on an interval, across every **track-mode** environment, under a single-writer lock. A
reconcile error is logged, observed, and notified — it never stops the loop.

### 15.1 Configure it

Every `daemon:` field defaults, so an existing config runs the daemon unchanged:

```yaml
daemon:
  interval: 60s                 # reconcile period; minimum 1s
  listen: "127.0.0.1:9713"      # HTTP bind for /healthz, /metrics, doorbell (localhost by default)
  reconcile_timeout: 5m         # per-environment reconcile deadline
  reconcile_failed_repeat: 1h   # suppress repeat reconcile_failed alerts per environment
  doorbell:
    enabled: false
```

### 15.2 What it reconciles

Only `source: { track: ... }` environments are auto-reconciled. **Promote targets are never
touched** — promotion stays a deliberate `gantry promote`. Each tick, per track-mode
environment, `serve` resolves latest releases, commits any pin diff, deploys + verifies,
records the outcome, and dispatches notifications. Releases are resolved **once per cycle**
and shared across environments (and a doorbell burst reuses that cached snapshot).

### 15.3 The single-writer lock

`serve` holds `<repo>/.gantry/serve.lock`. While held, the mutating CLI verbs
(`sync`/`deploy`/`promote`/`rollback`/`switch`) refuse:

```
Error: a gantry daemon is reconciling this repo (.gantry/serve.lock); retry when it is stopped
```

A lock whose owner is dead (or >24h old) is reclaimed automatically.

### 15.4 Topology: one writer clone, or `git.remote` sync

The lock is **per-filesystem**. The supported default is **one writer clone per repo**:
exactly one place runs `serve` and the CLI verbs against that clone. To run the daemon
somewhere other than the only clone, make it a fleet-safe worker with `git.remote` (§7.6):
it fast-forward-pulls at the top of each cycle and pushes after any cycle that committed.
gantry **never merges** — a non-fast-forward pull is a loud stop, not a merge.

### 15.5 Health, shutdown, reload

- `curl http://127.0.0.1:9713/healthz` → `ok` while running.
- `SIGINT`/`SIGTERM` → graceful shutdown (loop stops, HTTP drains 5s, lock released).
- `SIGHUP` → reload `gantry.yaml` without dropping the lock or the metrics registry; a bad
  edit is logged and the previous config keeps running.

### 15.6 systemd unit

```ini
# /etc/systemd/system/gantry.service
[Unit]
Description=gantry reconcile daemon
After=network-online.target

[Service]
Type=simple
User=gantry
WorkingDirectory=/srv/gantry
Environment=GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
ExecStart=/usr/local/bin/gantry serve --config /srv/gantry/gantry.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 15.7 The doorbell (webhook trigger)

A forge webhook can trigger an immediate (debounced) reconcile instead of waiting for the
interval:

```yaml
daemon:
  doorbell:
    enabled: true
    path: /hooks/forge
    secret: ${env:GANTRY_DOORBELL_TOKEN}  # required when enabled
    hmac: false   # true → verify GitHub's X-Hub-Signature-256 instead of a token header
```

The webhook carries **no version data** — it only says "something changed, go look"; a ring
cannot cause a deploy directly. Authentication is either a token header (`X-Gantry-Token` /
`X-Gitlab-Token`, constant-time compare — front with TLS since the secret transits the wire)
or an HMAC body signature (`hmac: true`, GitHub's `X-Hub-Signature-256`, secret never sent).
Bursts debounce to a single pending reconcile.

```bash
curl -XPOST -H "X-Gantry-Token: $GANTRY_DOORBELL_TOKEN" http://127.0.0.1:9713/hooks/forge  # → 202
```

### 15.8 Exposure & TLS

The daemon binds `127.0.0.1:9713` by default. `/metrics` reveals environment names and deploy
cadence; the doorbell authenticates a secret. To expose it (remote scrape, cross-host
webhook), either set `listen: "0.0.0.0:9713"` behind an authenticating/TLS-terminating
reverse proxy, or keep it on localhost and front it with nginx:

```nginx
server {
    listen 443 ssl;
    server_name gantry.example.com;
    ssl_certificate     /etc/letsencrypt/live/gantry.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/gantry.example.com/privkey.pem;
    location / { proxy_pass http://127.0.0.1:9713; proxy_set_header Host $host; }
}
```

---

## 16. Observability & metrics

gantry keeps two output channels separate:

- **Reports** (what you asked to see) → **stdout**.
- **Logs** (what gantry did) → **stderr**, structured via `slog`.

So `gantry status --all > matrix.txt` captures only the matrix, and
`gantry sync --env test --log-format json 2> gantry.log` captures only diagnostics. Two
persistent flags control the log: `--log-format text|json` (default text) and
`--log-level debug|info|warn|error` (default info).

### 16.1 The status matrix

```
COMPONENT   latest      test          prod
SVC_IMAGE   reg/svc:v9  reg/svc:v9    reg/svc:v8 !
PG_IMAGE    (untracked) postgres:16.2 postgres:16.2

test  ok  healthy  2h ago
prod  (no deploys)
```

A `!` marks an environment pin lagging the latest release (tracked components only).

### 16.2 Prometheus metrics (daemon)

The `serve` HTTP server exposes `/metrics`:

| metric | type | labels | meaning |
| --- | --- | --- | --- |
| `gantry_reconcile_total` | counter | `env`, `result` | reconcile passes: `deployed`/`failed`/`nochange` |
| `gantry_reconcile_duration_seconds` | histogram | `env` | wall-clock of one reconcile |
| `gantry_deploys_total` | counter | `env` | actual deploys (a reconcile with a real diff) |
| `gantry_verify_failures_total` | counter | `env` | post-deploy verify probes that failed |
| `gantry_rollbacks_total` | counter | `env`, `kind` | rollbacks; `kind="auto"` = verify-triggered |
| `gantry_drift_age_seconds` | gauge | `env` | age of the oldest drifted component, last observed |
| `gantry_build_info` | gauge | `version` | constant 1, carrying the version label |

```yaml
scrape_configs:
  - job_name: gantry
    static_configs:
      - targets: ["localhost:9713"]
```

Starter alert rules:

```yaml
groups:
  - name: gantry
    rules:
      - alert: GantryDriftStuck
        expr: gantry_drift_age_seconds > 86400
        for: 30m
        labels: { severity: warning }
        annotations: { summary: "gantry drift on {{ $labels.env }} > 24h" }
      - alert: GantryVerifyFailures
        expr: increase(gantry_verify_failures_total[15m]) > 0
        labels: { severity: warning }
        annotations: { summary: "gantry verify failing on {{ $labels.env }}" }
      - alert: GantryReconcileStalled
        expr: increase(gantry_reconcile_total{result="deployed"}[1h]) == 0 and increase(gantry_reconcile_total{result="nochange"}[1h]) == 0
        for: 1h
        labels: { severity: critical }
        annotations: { summary: "gantry reconcile loop appears stalled" }
```

---

## 17. CI integration recipes

gantry commits **locally** and does not push; in one-shot CI the runner must push the pin and
ledger commits so promotion/rollback on another machine see the same history.

### 17.1 GitHub Actions (one-shot on schedule)

```yaml
name: gantry-sync
on:
  schedule: [{ cron: "*/15 * * * *" }]
  workflow_dispatch:
jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: "1.26" }
      - run: go install github.com/RomanAgaltsev/gantry/cmd/gantry@latest
      - name: sync test
        env:
          GANTRY_FORGE_TOKEN: ${{ secrets.GANTRY_FORGE_TOKEN }}
        run: |
          echo "${{ secrets.DEPLOY_SSH_KEY }}" > /run/secrets/app_ssh_key
          gantry sync --env test --config gantry.yaml
          git push
      - name: drift gate
        run: gantry drift --all   # exit 3 fails the job
```

### 17.2 GitLab CI (auto test, manual prod)

```yaml
stages: [sync, promote]

sync-test:
  stage: sync
  image: golang:1.26
  rules: [{ if: '$CI_COMMIT_BRANCH == "main"' }]
  before_script:
    - go install github.com/RomanAgaltsev/gantry/cmd/gantry@latest
  script:
    - gantry sync --env test --config gantry.yaml
    - git push https://oauth2:${GIT_PUSH_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git HEAD:main

promote-prod:
  stage: promote
  image: golang:1.26
  when: manual
  needs: [sync-test]
  script:
    - go install github.com/RomanAgaltsev/gantry/cmd/gantry@latest
    - gantry promote --from test --to prod --config gantry.yaml
    - git push https://oauth2:${GIT_PUSH_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git HEAD:main
```

`--output json` on `status`/`history`/`drift`/`plan` gives machine-readable output for
gates and dashboards.

---

## 18. Day-2 operations runbook

Copy-paste procedures for the environment gantry manages.

### 18.1 Preconditions

- gantry runs **inside the orchestrator git repo** and commits to it.
- It **must own the working tree** — it refuses to act when unrelated changes are staged
  (`refusing to commit: "<path>" is already staged`). Commit or unstage first.
- It commits locally and does not push by default — push pin **and** ledger commits in CI,
  or use `git.remote` in the daemon.

### 18.2 Roll a new release into `test`

```bash
gantry plan    --env test    # read-only: pending pin changes + orphan pins
gantry sync    --env test    # pin latest, commit, deploy, verify, record
gantry history --env test    # confirm the ok/healthy outcome
```

### 18.3 Promote `test` → `prod`

```bash
gantry promote --from test --to prod --dry-run   # preview + gate decision
gantry promote --from test --to prod             # promote the latest GREEN test set
gantry promote --from test --to prod --sha <c>   # a specific commit (short SHA ok)
```

### 18.4 Roll `prod` back

```bash
gantry rollback --env prod --dry-run   # preview target set
gantry rollback --env prod             # restore last known-good, redeploy
```

### 18.5 Blue/green stage & switch

```bash
gantry sync     --env front   # stage on idle slot
gantry switch   --env front   # flip pointer (gated on ok deploy + idle verify)
gantry rollback --env front   # flip back
```

### 18.6 A deploy failed — now what?

A `sync`/`promote`/`rollback` whose deploy fails *after* its pin commit leaves the pins
committed but not running; gantry records `failed` and says so.

- **`test` (track mode):** rerun `gantry sync --env test` — it self-heals (redeploys the
  committed set, records a fresh outcome).
- **`prod` (promote target):** run `gantry deploy --env prod` — it reconciles the host to the
  committed pin file and records the new outcome.

Inspect at any time: `gantry history --env prod`. See everything: `gantry status --all`.

### 18.7 Recovering a `sync` whose deploy failed

`sync` commits new pins **before** deploying. If the deploy then fails, a plain re-`sync`
sees no diff and will not retry (track mode self-heals; a promote target needs the explicit
step above). The `sync` error tells you to run `gantry deploy --env <name>`, which reconciles
the host to the committed pin file — the supported recovery path.

---

## 19. Security model & hardening

### 19.1 Trust model: the config repo is trusted

`gantry.yaml` is operator-authored and lives in the git repo gantry commits to. **Anyone who
can write `gantry.yaml` — or the files and commands it references — controls the account
gantry runs as.** Treat write access to the config repo as equivalent to shell access on the
deploy host. Two consequences follow deliberately:

- **`${cmd:…}` is RCE-by-config by design.** It runs a host command; that is the feature (it
  is how a 1Password/SOPS/Vault CLI is shelled out to). It is acceptable *only* because the
  config is trusted — restrict who can edit it and what it points at.
- **Secret schemes inherit gantry's full environment.** `${cmd:…}`/`${sops:…}`/`${vault:…}`
  run as child processes inheriting `HOME`, `PATH`, `VAULT_ADDR`, etc. Run gantry as a
  dedicated, minimally-privileged user.

### 19.2 Safe defaults that hold regardless

- **No inline secret literals** — every credential is a `${scheme:arg}` SecretRef.
- **A referenced-but-unset secret is a hard error** — never a silent `""`.
- **Credentials ride stdin/child-env, not argv** — registry/vault tokens never appear in a
  `ps` snapshot.
- **`known_hosts` is required — no TOFU.** SSH executors reject an empty `known_hosts`.
- **The doorbell secret is compared constant-time (token) or verified via HMAC-SHA256.**

### 19.3 Hardening checklist

- Run gantry as a dedicated OS user with only the SSH keys it needs.
- Keep `daemon.listen` on `127.0.0.1` and front `/metrics` + the doorbell with TLS.
- Prefer digest pins (publish `image_digest`) so a re-pushed tag cannot drift a host.
- Scope the forge token to `read_api` (GitLab) / release-read (GitHub) only.
- Restrict write access to the config repo; review `gantry.yaml` changes like code.

---

## 20. CLI command reference

All commands take `--config <path>` (default `gantry.yaml`). Read verbs support
`--output json` (short `-o`). Persistent flags: `--log-format text|json`,
`--log-level debug|info|warn|error`.

| Command | Reads forge? | Writes pin? | Deploys? | Purpose |
| --- | --- | --- | --- | --- |
| `plan --env <e>` | yes | no | no | Show pending pin changes (a `sync --dry-run`) + orphan pins. |
| `sync --env <e>` | yes | on diff | on diff | Resolve latest releases, commit changed pins, deploy, verify, record. Skips explicit pins. |
| `deploy --env <e>` | no | no | yes | Reconcile the host to the **whole committed pin file** (every component). Recovery / post-Renovate / post-promote path. |
| `status [--env <e> | --all]` | yes | no | no | Pinned vs. latest per component; `--all` = matrix; `--watch --interval <d>` for a live view. |
| `diff --env <a> --to <b>` | no | no | no | Pin differences between two environments. |
| `promote --from <a> --to <b>` | no | commits | yes | Copy a frozen green pin set upstream→downstream, gated on the ledger. `--sha`, `--only`, `--dry-run`. |
| `rollback --env <e>` | no | commits | yes | Restore the last known-good set. `--fast` (symlink-release), `--dry-run`. |
| `switch --env <e>` | no | no | yes | Blue-green: flip the pointer to the staged slot, gated on an ok deploy + idle verify. |
| `history --env <e>` | no | no | no | Ledger entries for an environment. |
| `drift [--env <e> | --all]` | yes | no | no | Report unconsumed releases past the threshold. Exit 3 on drift. |
| `prune --env <e>` | no | commits | yes | Remove orphan pins and redeploy the reduced set. `--dry-run`. |
| `validate` | no | no | no | Schema + secret-resolution + pin-file consistency check. |
| `serve` | yes | on diff | on diff | Run the reconcile loop as a daemon. `--interval <d>`. |
| `version` | no | no | no | Print the gantry version. |

**Global exit codes:** `0` success · `3` drift detected (a CI affordance, distinct from
failure) · `2` usage error · `1` any other operational error.

---

## 21. Troubleshooting & FAQ

**`Error: a gantry daemon is reconciling this repo`** — a `serve` process holds the
single-writer lock. Stop the daemon (or wait); a stale lock (dead owner / >24h) is reclaimed
automatically.

**`refusing to commit: "<path>" is already staged`** — gantry builds commits from the index
and refuses when unrelated changes are staged. Commit or unstage them first.

**A release is skipped / hard error on sync** — the release is missing or has an invalid
metadata block (§8), or its `semver_version` marks it a prerelease. gantry never silently
skips releases; fix the metadata block or publish a stable release.

**A referenced secret errors** — a `${env:…}`/`${file:…}`/vault field is unset/missing.
gantry fails loudly close to the cause rather than substituting `""`. Provide it.

**Deploy failed after the pin commit; re-`sync` does nothing** — track mode self-heals on the
next `sync`; a promote target needs `gantry deploy --env <e>` (§18.6/18.7).

**Rollback refuses** — there is no earlier green deploy on record for that environment.

**`sops`/`vault` secret fails in the container** — the distroless image lacks those binaries.
Use `${vault-http:…}`, a fatter image, or resolve the secret in CI (§13.3).

**Host key rejected** — the host's key is not in the `known_hosts` you supplied. Add it with
`ssh-keyscan -H <host>` and update the referenced file. gantry never trusts unknown keys.

**Pushed nothing / another machine can't see the promotion** — gantry commits locally only.
Push the pin and ledger commits (CI `git push`), or enable `git.remote` on the daemon.

---

## Reference pages (deeper dives)

- [Getting started](getting-started.md) · [Configuration reference](configuration.md)
- [Executors](executors.md) · [Blue-green](blue-green.md) · [Verification](verification.md)
- [Promotion](promotion.md) · [Drift](drift.md)
- [Daemon](daemon.md) · [Observability](observability.md)
- [Secrets](secrets.md) · [Notifications](notifications.md) · [Runbook](runbook.md)
- [Security model](security.md) · [Developer guide](developer-guide.md)
