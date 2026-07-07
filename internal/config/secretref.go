package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SecretRef is an indirection to a secret; a literal value is rejected at resolve time.
type SecretRef struct{ Raw string }

// UnmarshalYAML captures the scalar value verbatim.
func (s *SecretRef) UnmarshalYAML(value *yaml.Node) error {
	s.Raw = value.Value
	return nil
}

// SecretResolver resolves SecretRefs against the environment and filesystem.
type SecretResolver struct {
	LookupEnv func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
	// Runner runs a host command (sops/vault/cmd) under ctx, with env appended to the child's
	// environment. It returns trimmed-able stdout; a failed command surfaces its stderr.
	Runner func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	// Vault holds the resolved ambient Vault address/token used by ${vault:…} (Task 5).
	Vault VaultDefaults

	// schemes holds per-resolver scheme overrides; nil means only the built-ins apply. Populated
	// via WithScheme so there is no shared mutable state (review §2.2-D).
	schemes map[string]SchemeFunc
}

// VaultDefaults are the resolved ambient Vault address and token used by ${vault:…}. Both
// empty means no Vault defaults are configured; ${vault:…} then uses the vault CLI's own
// ambient env (VAULT_ADDR/VAULT_TOKEN).
type VaultDefaults struct {
	Address string
	Token   string
}

// DefaultResolver resolves against the real OS environment and filesystem.
func DefaultResolver() SecretResolver {
	return SecretResolver{LookupEnv: os.LookupEnv, ReadFile: os.ReadFile, Runner: execRunner}
}

// execRunner runs name with args under ctx, env appended to os.Environ, capturing stdout and
// surfacing stderr on failure.
func execRunner(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// runnerTimeout bounds a runner-backed scheme so a hung sops/vault/cmd cannot wedge a
// gantry command or a daemon reconcile.
const runnerTimeout = 30 * time.Second

// run runs name with args under a bounded context derived from ctx with no extra env
// (used by cmd/sops). The caller's ctx bounds it on top of runnerTimeout (whichever fires
// first), so a daemon shutting down cancels an in-flight command.
func (r SecretResolver) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, runnerTimeout)
	defer cancel()
	return r.Runner(cctx, nil, name, args...)
}

// runWithEnv runs name with args under a bounded context derived from ctx with env appended to
// the child env (used by vault to pass VAULT_TOKEN off the process arg list).
func (r SecretResolver) runWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, runnerTimeout)
	defer cancel()
	return r.Runner(cctx, env, name, args...)
}

// resolveCmd runs a command and returns its trimmed stdout as the secret. The arg is split
// on whitespace: "${cmd:prog a b}" runs prog with args [a b]. Commands needing shell quoting
// should be wrapped in a script.
func resolveCmd(ctx context.Context, r SecretResolver, arg string) (string, error) {
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return "", errors.New("cmd secret: empty command")
	}
	out, err := r.run(ctx, fields[0], fields[1:]...)
	if err != nil {
		return "", fmt.Errorf("cmd secret %q: %w", fields[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveSOPS decrypts a SOPS file via the sops binary and extracts a dotted key. Arg form:
// "path#dotted.key"; with no "#key" the whole trimmed decrypted output is the secret.
func resolveSOPS(ctx context.Context, r SecretResolver, arg string) (string, error) {
	file, key, hasKey := strings.Cut(arg, "#")
	out, err := r.run(ctx, "sops", "-d", file)
	if err != nil {
		return "", fmt.Errorf("sops decrypt %q: %w", file, err)
	}
	if !hasKey {
		return strings.TrimSpace(string(out)), nil
	}
	var doc map[string]any
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return "", fmt.Errorf("sops %q: parse decrypted YAML: %w", file, err)
	}
	v, err := walkDotted(doc, key)
	if err != nil {
		return "", fmt.Errorf("sops %q: %w", file, err)
	}
	return v, nil
}

// walkDotted follows a dotted path into a decoded YAML/JSON map to a scalar leaf.
func walkDotted(doc map[string]any, path string) (string, error) {
	var cur any = doc
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("key %q: %q is not a map", path, part)
		}
		cur, ok = m[part]
		if !ok {
			return "", fmt.Errorf("key %q not found", path)
		}
	}
	return fmt.Sprintf("%v", cur), nil
}

// resolveVault reads a field from a Vault KV secret via the vault binary. Arg form:
// "mount/path#field". Address/token come from the resolver's VaultDefaults; the token is
// passed via the child env (VAULT_TOKEN) so it stays off the process arg list.
func resolveVault(ctx context.Context, r SecretResolver, arg string) (string, error) {
	path, field, ok := strings.Cut(arg, "#")
	if !ok {
		return "", fmt.Errorf("vault secret %q: want mount/path#field", arg)
	}
	args := []string{"kv", "get", "-format=json"}
	if r.Vault.Address != "" {
		args = append(args, "-address="+r.Vault.Address)
	}
	args = append(args, path)

	env := []string{}
	if r.Vault.Token != "" {
		env = append(env, "VAULT_TOKEN="+r.Vault.Token)
	}
	out, err := r.runWithEnv(ctx, env, "vault", args...)
	if err != nil {
		return "", fmt.Errorf("vault kv get %q: %w", path, err)
	}
	var resp struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("vault %q: parse response: %w", path, err)
	}
	v, ok := resp.Data.Data[field]
	if !ok {
		return "", fmt.Errorf("vault %q: field %q not found", path, field)
	}
	return v, nil
}

// SchemeFunc resolves the arg of a ${scheme:arg} ref. ctx is the caller's context (a
// shelling-out scheme must honor its cancellation); res is passed so a backend can compose
// other schemes (e.g. vault resolving its own token) and so tests can inject fakes.
type SchemeFunc func(ctx context.Context, res SecretResolver, arg string) (string, error)

// builtinSchemes are the always-available schemes. It is written once at init and never
// mutated (no Register), so there is no shared mutable state; per-resolver additions go
// through WithScheme instead (review §2.2-D).
var builtinSchemes = map[string]SchemeFunc{
	"env":   resolveEnv,
	"file":  resolveFile,
	"cmd":   resolveCmd,
	"sops":  resolveSOPS,
	"vault": resolveVault,
}

// WithScheme returns a copy of the resolver with an added/overridden scheme (used by tests
// and future backends such as a ${vaultlite:…} follow-up) — no package-global mutation.
func (r SecretResolver) WithScheme(name string, fn SchemeFunc) SecretResolver {
	cp := make(map[string]SchemeFunc, len(r.schemes)+1)
	for k, v := range r.schemes {
		cp[k] = v
	}
	cp[name] = fn
	r.schemes = cp
	return r
}

// schemeFor resolves a scheme name against the per-resolver overrides, then the built-ins.
func (r SecretResolver) schemeFor(name string) (SchemeFunc, bool) {
	if fn, ok := r.schemes[name]; ok {
		return fn, true
	}
	fn, ok := builtinSchemes[name]
	return fn, ok
}

// Resolve returns the secret value for a ref. Empty ref → "". Inline (non-${...}) → error.
// ctx bounds any shelling-out scheme (cmd/sops/vault) on top of the per-scheme timeout, so a
// daemon shutting down cancels an in-flight `vault kv get`.
func (r SecretResolver) Resolve(ctx context.Context, s SecretRef) (string, error) {
	raw := strings.TrimSpace(s.Raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "${") || !strings.HasSuffix(raw, "}") {
		return "", errors.New("inline secret not allowed; use ${env:NAME} or ${file:/path}")
	}
	inner := raw[2 : len(raw)-1]
	scheme, arg, ok := strings.Cut(inner, ":")
	if !ok {
		return "", fmt.Errorf("malformed secret ref %q", raw)
	}
	fn, ok := r.schemeFor(scheme)
	if !ok {
		return "", fmt.Errorf("unknown secret scheme %q", scheme)
	}
	return fn(ctx, r, arg)
}

// resolveEnv reads an environment variable. A referenced-but-unset variable is a
// configuration error, not an empty secret: silently resolving to "" turns a typo'd
// ${env:NAME} into an unauthenticated request or an empty docker-login password that fails
// far from the cause. An explicitly set-but-empty variable still resolves to "".
func resolveEnv(_ context.Context, r SecretResolver, arg string) (string, error) {
	v, ok := r.LookupEnv(arg)
	if !ok {
		return "", fmt.Errorf("secret env var %q is referenced but not set", arg)
	}
	return v, nil
}

// resolveFile reads a file and returns its trimmed contents.
func resolveFile(_ context.Context, r SecretResolver, arg string) (string, error) {
	b, err := r.ReadFile(arg)
	if err != nil {
		return "", fmt.Errorf("read secret file %q: %w", arg, err)
	}
	return strings.TrimSpace(string(b)), nil
}
