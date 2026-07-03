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

// resolveSOPS decrypts a SOPS file via the sops binary and extracts a dotted key. Arg form:
// "path#dotted.key"; with no "#key" the whole trimmed decrypted output is the secret.
func resolveSOPS(r SecretResolver, arg string) (string, error) {
	file, key, hasKey := strings.Cut(arg, "#")
	out, err := r.run("sops", "-d", file)
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

// SchemeFunc resolves the arg of a ${scheme:arg} ref. res is passed so a backend can compose
// other schemes (e.g. vault resolving its own token) and so tests can inject fakes.
type SchemeFunc func(res SecretResolver, arg string) (string, error)

// schemes maps a scheme name to its resolver. env/file are built in; cmd/sops/vault register
// themselves in their own tasks. Register adds or overrides an entry (tests, future backends).
var schemes = map[string]SchemeFunc{
	"env":  resolveEnv,
	"file": resolveFile,
	"cmd":  resolveCmd,
	"sops": resolveSOPS,
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
