# Configuration reference

gantry is driven entirely by a single YAML file (by default `gantry.yaml`, override
with `--config`). This document describes every field, the secret-reference schemes,
and the environment source model.

The configuration is read, defaulted, and validated by `config.Load`. A validation
error stops the command before any forge call or deploy.

## Top-level structure

```yaml
forge:        { ... }            # required — which forge to read releases from
connections:  { <name>: { ... } } # named deploy targets (inventory)
components:   [ { ... } ]         # the buildable repos whose images are pinned
environments: [ { ... } ]         # deploy-target environments
registries:   { <host>: { ... } } # optional — private registry credentials
```

## `forge`

Selects and configures the forge adapter that releases are read from.

| Field             | Type        | Required | Description |
|-------------------|-------------|----------|-------------|
| `kind`            | string      | yes      | Forge adapter. Slice 1 supports `gitlab` only; any other value is a validation error. |
| `base_url`        | string      | yes      | Base URL of the forge, e.g. `https://gitlab.example.com`. |
| `token`           | SecretRef   | yes      | API token. GitLab needs `read_api` scope. See [SecretRef](#secretref). |
| `metadata_marker` | string      | no       | Namespace of the release-metadata block embedded in release descriptions. Defaults to `gantry-release-metadata`. |

### The release-metadata block

gantry does not guess image references from tag names. Each release description must
embed a metadata block delimited by the configured marker:

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

`image_repository` and `image_tag` are required; `built_at` must be RFC 3339. A
release with a missing or invalid metadata block is a **hard error** — gantry never
silently skips a release.

## `connections`

A map of named deploy targets (the inventory). Environments reference a connection
by name through `executor.connection`.

```yaml
connections:
  app-host:
    address: 192.0.2.10
    ssh:
      user: deploy
      key: ${file:/run/secrets/app_ssh_key}
      known_hosts: ${file:/run/secrets/known_hosts}
```

| Field          | Type      | Required | Description |
|----------------|-----------|----------|-------------|
| `address`      | string    | yes      | Host address. A bare host gets the default SSH port (22); `host:port` is honored. |
| `ssh.user`     | string    | yes\*    | SSH login user. |
| `ssh.key`      | SecretRef | yes\*    | PEM-encoded SSH private key. |
| `ssh.known_hosts` | SecretRef | yes\* | Contents of a `known_hosts` file. **Required** — gantry rejects unknown host keys rather than trusting them on first use. |

\* Required when the `compose-over-ssh` executor connects to this host.

## `components`

The list of buildable repos whose images are pinned.

```yaml
components:
  - { id: api, project: demo/api, pin_key: API_IMAGE }
  - { id: web, project: demo/web, pin_key: WEB_IMAGE }
```

| Field     | Type   | Required | Description |
|-----------|--------|----------|-------------|
| `id`      | string | yes      | Human-readable identifier for the component. |
| `project` | string | yes      | Forge project path (e.g. `group/repo`) or numeric ID. |
| `pin_key` | string | yes      | The dotenv key the resolved `repository:tag` is written under. Must be unique across components. |

A duplicate `pin_key` is a validation error.

## `environments`

Each environment is one deploy target.

```yaml
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

| Field      | Type            | Required | Description |
|------------|-----------------|----------|-------------|
| `name`     | string          | yes      | Environment name, used by `--env`. |
| `source`   | Source          | yes      | How this environment's pins are computed (see below). |
| `pin_file` | string          | yes      | Path (relative to the git working tree) of the dotenv pin file gantry maintains. |
| `executor` | ExecutorConfig  | yes      | Deploy backend (see below). |

### `source`

Declares where an environment's desired pins come from.

| Field          | Type   | Description |
|----------------|--------|-------------|
| `track`        | string | Track the forge directly, e.g. `latest` — pin the latest release of each component. |
| `promote_from` | string | Name of an upstream environment to copy pins from instead of tracking the forge. |

An environment must set **either** `track` **or** `promote_from`; setting neither is a
validation error. If `promote_from` names an environment that does not exist, that is
also an error.

> **Slice 1 note:** `promote_from` is modeled and validated, but its execution is
> **not** implemented yet — `sync` supports track-mode only. Promotion is planned for
> a later slice.

### `executor`

| Field           | Type     | Required | Description |
|-----------------|----------|----------|-------------|
| `kind`          | string   | yes      | Deploy backend. Slice 1 supports `compose-over-ssh` only; any other value is a validation error. |
| `connection`    | string   | yes      | Name of a `connections` entry. Must exist, else a validation error. |
| `project_dir`   | string   | yes      | Directory on the target host that holds the compose project. |
| `compose_files` | []string | no       | Compose files passed as `-f` flags. |
| `env_file`      | string   | no       | Name of the env file written into `project_dir` and passed as `--env-file`. |

The executor writes the rendered pin set to `project_dir/env_file` on the host, then
runs, scoped to the project dir and compose files:

```
docker compose -f <each compose file> --env-file <env_file> pull
docker compose -f <each compose file> --env-file <env_file> up -d
```

## `registries`

Optional. A map keyed by **registry host** of credentials used to `docker login` on
the target host before pulling. Before `docker compose pull`, gantry inspects the
pinned image references, determines their registry hosts, and logs in only to the
registries actually referenced by the pin set.

```yaml
registries:
  gitlab.example.com:5050:
    user: ${env:GANTRY_REGISTRY_USER}
    password: ${env:GANTRY_REGISTRY_TOKEN}
```

| Field      | Type      | Description |
|------------|-----------|-------------|
| `user`     | SecretRef | Registry username. |
| `password` | SecretRef | Registry password/token (fed to `docker login --password-stdin`). |

The registry host is derived from each image reference using Docker's rule: the first
path segment is the host if it contains `.` or `:` or equals `localhost`; otherwise
the image lives on `docker.io`. For example `gitlab.example.com:5050/g/s:v1` →
`gitlab.example.com:5050`, while `postgres:16.4` → `docker.io`.

## SecretRef

Every credential field (`forge.token`, `ssh.key`, `ssh.known_hosts`, `registries.*.user`,
`registries.*.password`) is a **SecretRef**, never an inline literal. A SecretRef is a
string with one of these schemes:

| Scheme            | Resolves to |
|-------------------|-------------|
| `${env:NAME}`     | The value of environment variable `NAME`. |
| `${file:/path}`   | The contents of the file at `/path`, trimmed of surrounding whitespace. |

An empty value resolves to the empty string. **Any other (non-`${...}`) value is an
error** — gantry refuses to read inline secrets out of the config file. This keeps
credentials out of version control.

```yaml
token: ${env:GANTRY_FORGE_TOKEN}      # OK
key:   ${file:/run/secrets/ssh_key}   # OK
token: glpat-literal-token            # ERROR: inline secret not allowed
```
