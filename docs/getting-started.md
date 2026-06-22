# Getting started

This guide takes you from nothing to a working `gantry sync` against a test
environment. gantry consumes the latest release of each component from a forge,
pins the resolved image references into a per-environment dotenv file (committing
on change), and deploys the environment with `docker compose pull && up -d` over SSH.

## 1. Install

### Build from source

You need Go 1.26 or newer.

```bash
git clone https://github.com/RomanAgaltsev/gantry.git
cd gantry
task build            # produces ./gantry
# or, without Task:
go build -o gantry ./cmd/gantry
```

Check the build:

```bash
./gantry version
```

### Run in Docker

The repository ships a distroless image build:

```bash
docker build -t gantry:dev --build-arg VERSION=dev .
docker run --rm gantry:dev version
```

Because gantry reads `gantry.yaml`, resolves secrets from the environment or
mounted files, and commits to a git working tree, mount those in when you run it:

```bash
docker run --rm \
  -e GANTRY_FORGE_TOKEN \
  -v "$PWD:/work" -w /work \
  -v /run/secrets:/run/secrets:ro \
  gantry:dev sync --env test --config gantry.yaml
```

## 2. Write `gantry.yaml`

A minimal single-component configuration:

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

Notes:

- Secrets are never written inline. Every secret field is a reference of the form
  `${env:NAME}` or `${file:/path}` — an inline literal is rejected. See
  [configuration.md](configuration.md#secretref).
- `known_hosts` is required for SSH: gantry does not silently trust unknown host
  keys.
- The directory containing `pin_file` must be a git working tree — gantry commits
  the pin file there when it changes.

The configuration above lives complete and runnable in
[`examples/demo/gantry.yaml`](../examples/demo/gantry.yaml).

## 3. Provide the forge token

`gantry.yaml` references the token via `${env:GANTRY_FORGE_TOKEN}`, so export it:

```bash
export GANTRY_FORGE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

The token needs read access to the project's releases (GitLab `read_api` scope).

## 4. Plan (read-only)

`plan` shows the pin changes a `sync` would make, without writing or deploying:

```bash
./gantry plan --env test --config gantry.yaml
```

Sample output:

```
API_IMAGE: reg/api:v1.3.0 -> reg/api:v1.4.0
```

If everything is already pinned to the latest release, it prints `up to date; no
changes`.

## 5. Sync (pin + deploy)

`sync` resolves the latest releases, and if the pins differ from what is currently
recorded it:

1. writes the updated `pin_file` and commits it to git (commit-on-diff);
2. writes the env file on the target host over SSH;
3. logs in to any private registries the images reference;
4. runs `docker compose pull` then `docker compose up -d`.

```bash
./gantry sync --env test --config gantry.yaml
```

If nothing changed, `sync` is a no-op: no commit, no deploy.

## 6. Status

`status` compares the currently pinned image of each component against the latest
release available on the forge, without changing anything:

```bash
./gantry status --env test --config gantry.yaml
```

```
API_IMAGE            pinned=reg/api:v1.3.0          latest=reg/api:v1.4.0
```

## Next steps

- Read the full [configuration reference](configuration.md).
- Walk through the [demo example](../examples/demo/README.md).
