# Executors

An executor is *how* gantry reconciles a host to a pin set. The kind is per environment
(`executor.kind`); both kinds run over the same SSH connection.

## `compose-over-ssh`

Writes the env file in place and runs `docker compose pull && up -d`. The minimal
primitive — use it when you do not need versioned releases or instant rollback.

```yaml
executor:
  kind: compose-over-ssh
  connection: app-host
  project_dir: /opt/app
  compose_files: [compose.yaml]
  env_file: .env.versions.prod
```

## `symlink-release`

Each deploy lands in a **new versioned directory** named by the pin commit, and an atomic
`current` symlink is flipped to it:

```
/opt/app/
  releases/
    abc1234/   .env  .version     <- a past release
    def5678/   .env  .version     <- the new release
  current -> releases/def5678     <- atomically flipped (mv -T rename)
```

The stack runs from `current/.env`. Two properties this buys you:

- **Atomic config swap** — `current` flips with a single rename; there is no window where
  the env file is half-written.
- **Instant rollback** — `gantry rollback` writes the previous pin set as a new commit and
  redeploys; because the old images are already on the host, the flip-and-`up` is fast.

```yaml
executor:
  kind: symlink-release
  connection: app-host
  project_dir: /opt/app
  compose_files: [compose.yaml]
  keep: 10          # retain the newest 10 release dirs; 0 (default) keeps all
```

`symlink-release` needs no `env_file` (it always uses `current/.env`).

### Retention: `keep: N`

Each deploy adds a `releases/<commit>` directory. Set `keep: N` to bound how many are
retained: after a successful deploy the oldest release directories beyond the newest `N` are
removed, so years of daemon operation does not accumulate thousands of small directories.

- `keep: 0` (the default) **keeps all** releases — today's behavior.
- The **active release** (the one `current` points at) is **never pruned**, even when it falls
  outside the newest `N` (for example after a rollback to an older release).
- Pruning runs after a successful deploy and is **best-effort**: a prune failure is logged and
  skipped, never failing an otherwise-good deploy.

The release directory name is the **pin commit SHA**, so it matches the ledger entry for
that deploy — `gantry history` and the directory under `releases/` line up one-to-one.
