package config

import (
	"bytes"
	"context"
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

// run runs name with args under a bounded context with no extra env (used by cmd/sops).
func (r SecretResolver) run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runnerTimeout)
	defer cancel()
	return r.Runner(ctx, nil, name, args...)
}

// resolveCmd runs a command and returns its trimmed stdout as the secret. The arg is split
// on whitespace: "${cmd:prog a b}" runs prog with args [a b]. Commands needing shell quoting
// should be wrapped in a script.
func resolveCmd(r SecretResolver, arg string) (string, error) {
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		return "", errors.New("cmd secret: empty command")
	}
	out, err := r.run(fields[0], fields[1:]...)
	if err != nil {
		return "", fmt.Errorf("cmd secret %q: %w", fields[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SchemeFunc resolves the arg of a ${scheme:arg} ref. res is passed so a backend can compose
// other schemes (e.g. vault resolving its own token) and so tests can inject fakes.
type SchemeFunc func(res SecretResolver, arg string) (string, error)

// schemes maps a scheme name to its resolver. env/file are built in; cmd/sops/vault register
// themselves in their own tasks. Register adds or overrides an entry (tests, future backends).
var schemes = map[string]SchemeFunc{
	"env":  resolveEnv,
	"file": resolveFile,
	"cmd":  resolveCmd,
}

// Register adds or overrides a secret scheme. Intended for tests and future backends
// (e.g. a ${vaultlite:…} follow-up).
func Register(scheme string, fn SchemeFunc) { schemes[scheme] = fn }

// Resolve returns the secret value for a ref. Empty ref → "". Inline (non-${...}) → error.
func (r SecretResolver) Resolve(s SecretRef) (string, error) {
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
	fn, ok := schemes[scheme]
	if !ok {
		return "", fmt.Errorf("unknown secret scheme %q", scheme)
	}
	return fn(r, arg)
}

// resolveEnv reads an environment variable. A referenced-but-unset variable is a
// configuration error, not an empty secret: silently resolving to "" turns a typo'd
// ${env:NAME} into an unauthenticated request or an empty docker-login password that fails
// far from the cause. An explicitly set-but-empty variable still resolves to "".
func resolveEnv(r SecretResolver, arg string) (string, error) {
	v, ok := r.LookupEnv(arg)
	if !ok {
		return "", fmt.Errorf("secret env var %q is referenced but not set", arg)
	}
	return v, nil
}

// resolveFile reads a file and returns its trimmed contents.
func resolveFile(r SecretResolver, arg string) (string, error) {
	b, err := r.ReadFile(arg)
	if err != nil {
		return "", fmt.Errorf("read secret file %q: %w", arg, err)
	}
	return strings.TrimSpace(string(b)), nil
}
