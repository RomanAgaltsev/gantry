# Security model

gantry is a deploy orchestrator: it reads a config file from a git repo, resolves secrets,
talks to a forge, and runs commands over SSH on deploy hosts. This page states the threat
model that shape follows, so operators can decide where to trust it and where to harden the
boundary around it.

## Trust model: the config repo is trusted

`gantry.yaml` is **operator-authored** and lives in the git repo gantry already commits to
(the pin file, the ledger). Anyone who can write `gantry.yaml` — or the files and commands it
references — **controls the account gantry runs as**. Treat write access to the config repo as
equivalent to shell access on the deploy host.

This is deliberate and is the basis for two behaviors worth stating explicitly:

- **`${cmd:…}` is RCE-by-config by design (S2).** A `${cmd:prog arg}` secret runs `prog` with
  `arg` on the gantry host. That is the feature (it is how a 1Password/SOPS/Vault CLI is
  shelled out to), not an oversight. It is acceptable *only* because the config is trusted —
  restrict who can edit `gantry.yaml` and the files/commands it points at.
- **Secret schemes inherit gantry's full environment (S4).** `${cmd:…}`, `${sops:…}`, and
  `${vault:…}` run as child processes and inherit the parent environment (`HOME`, `PATH`,
  `VAULT_ADDR`, …) so the CLIs find their own config. This is required for the tools to work;
  it is why the gantry process should run as a dedicated, minimally-privileged user.

## Safe defaults that hold regardless

The trust model narrows the attack surface to the config author; the resolution layer then
hardens what an author can express:

- **No inline secret literals.** Every credential is a `${scheme:arg}` `SecretRef`; a literal
  password in `gantry.yaml` is not a supported shape. Secrets come from `env`/`file`/`cmd`/
  `sops`/`vault`.
- **A referenced-but-unset secret is a hard error.** gantry never silently substitutes `""`
  for a missing env var, file, or vault field — it fails loudly, close to the cause.
- **Credentials ride stdin/child-env, not argv.** Registry and vault tokens are passed to the
  relevant tools via stdin or a child process's environment, never on the process arg list
  (where a `ps` snapshot would leak them).
- **`known_hosts` is required — no TOFU.** SSH executors reject an empty `known_hosts`; there
  is no silent trust-on-first-use of an unseen host key. (See the [note on the known_hosts
  temp file](#known_hosts-and-the-temp-file) below.)
- **Inline secret literals in the config are rejected** and the doorbell secret is compared
  constant-time (token header) or verified with HMAC-SHA256 (`X-Hub-Signature-256`, see
  [daemon.md](daemon.md#doorbell)).

## known_hosts and the temp file

gantry requires the *contents* of a `known_hosts` file (passed as a `${file:…}` `SecretRef`),
not a path. `golang.org/x/crypto`'s `knownhosts.New` only parses from a file path, so gantry
writes those contents to a short-lived temp file, parses it, and removes it immediately (S3).
The material written is **public host keys** (not a private key), and the window is momentary.
The temp file is created mode `0600` defensively (so a permissive `umask` cannot leave it
world-readable). If a future `x/crypto` release exposes an in-memory parser, the temp file can
be dropped — the awkwardness lives in the upstream seam, not in gantry's choice to require host
key pinning.

## Daemon exposure

The daemon binds `127.0.0.1:9713` by default (S1). `/metrics` reveals environment names and
deploy cadence; the doorbell authenticates a secret. See
[daemon.md — Exposure & TLS](daemon.md#exposure--tls) for how to expose it safely (a
TLS-terminating reverse proxy) and [daemon.md — Doorbell](daemon.md#doorbell) for HMAC body
signatures that keep the secret off the wire.
