# Secret backends

Every credential in `gantry.yaml` (the forge token, SSH key/known_hosts, registry
user/password, SMTP password, doorbell secret) is a **SecretRef** — a `${scheme:arg}` string
that gantry resolves at use time, never an inline literal. `${env:…}` and `${file:…}` are
built in; `${cmd:…}`, `${sops:…}`, and `${vault:…}` shell out to host tools through an
injectable runner.

A missing secret is **always an error** — gantry never silently substitutes an empty string
(a referenced-but-unset env var, a missing file, an unknown vault field all fail loudly,
close to the cause).

## Schemes

| scheme | form | reads |
| --- | --- | --- |
| `env` | `${env:NAME}` | environment variable (unset = error; set-but-empty = `""`) |
| `file` | `${file:/path}` | file contents, trimmed |
| `cmd` | `${cmd:prog a b}` | command stdout, trimmed (`prog` run with args `[a b]`) |
| `sops` | `${sops:file.enc.yaml#db.password}` | a dotted key from a SOPS-decrypted file |
| `vault` | `${vault:secret/gantry#field}` | a field from a Vault KV secret |

Examples:

```yaml
forge:
  token: ${env:GANTRY_FORGE_TOKEN}                 # built in
connections:
  host:
    ssh:
      key: ${file:/run/secrets/ssh_key}            # built in
      known_hosts: ${file:/run/secrets/known_hosts}
registries:
  registry.example.com:
    user: ${cmd:op read op://vault/reg/user}        # 1Password CLI → stdout
    password: ${sops:secrets.enc.yaml#reg.password} # Mozilla SOPS file + dotted key
    # password: ${vault:secret/gantry#reg_password} # HashiCorp Vault KV
```

- `${cmd:…}` splits its arg on whitespace and runs `prog a b`; commands that need shell
  quoting should be wrapped in a script.
- `${cmd:…}` runs an arbitrary host command by design, and `${cmd:…}`/`${sops:…}`/`${vault:…}`
  inherit gantry's full environment. The config is trusted — see the
  [security model](security.md) for why that is acceptable and what it implies for who may edit
  `gantry.yaml`.
- `${sops:path}` with **no** `#key` returns the whole trimmed decrypted output as the secret.
- `${vault:mount/path#field}` reads a single field; the Vault address/token come from the
  `secrets.vault` block (below).

## `secrets.vault` (Vault defaults)

`${vault:…}` needs the Vault address and token. Set them once under the optional
`secrets.vault` block — each value is itself a `SecretRef` (so the token can come from env
or a file):

```yaml
secrets:
  vault:
    address: ${env:VAULT_ADDR}
    token:   ${env:VAULT_TOKEN}   # or ${file:/run/secrets/vault_token}
```

Both default to `${env:VAULT_ADDR}` / `${env:VAULT_TOKEN}`, so if those env vars are set on
the host you need no `secrets.vault` block at all. The token is passed to the `vault` CLI via
its environment (`VAULT_TOKEN`), not the process arg list.

## Binary-dependency caveat

`env` and `file` are pure Go and work everywhere. `${cmd:…}`, `${sops:…}`, and `${vault:…}`
shell out to the **`cmd`/`sops`/`vault` binaries on the host** — gantry does not vendor them.
The default distroless runtime image ships only `env`/`file`/`cmd` (when the `cmd` binary
exists), so to use `sops`/`vault` either run gantry in a fatter image with those CLIs
installed or resolve those secrets in CI before invoking gantry. A runner call is bounded to
30s so a hung tool cannot wedge a command or a daemon reconcile.

## Extending it

The built-in scheme set (`env`, `file`, `cmd`, `sops`, `vault`) is fixed and immutable.
A resolver carries its own per-instance overrides via `WithScheme`, which returns a copy with
the scheme added (no shared mutable state):

```go
res := config.DefaultResolver().WithScheme("vaultlite", func(ctx context.Context, r config.SecretResolver, arg string) (string, error) {
    // an in-process Vault client, no CLI
})
```

So a future `${vaultlite:…}` backend is a one-function addition behind the same seam.
