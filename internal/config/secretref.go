package config

import (
	"fmt"
	"os"
	"strings"

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
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
}

// DefaultResolver resolves against the real OS environment and filesystem.
func DefaultResolver() SecretResolver {
	return SecretResolver{Getenv: os.Getenv, ReadFile: os.ReadFile}
}

// Resolve returns the secret value for a ref. Empty ref → "". Inline (non-${...}) → error.
func (r SecretResolver) Resolve(s SecretRef) (string, error) {
	raw := strings.TrimSpace(s.Raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "${") || !strings.HasSuffix(raw, "}") {
		return "", fmt.Errorf("inline secret not allowed; use ${env:NAME} or ${file:/path}")
	}
	inner := raw[2 : len(raw)-1]
	scheme, arg, ok := strings.Cut(inner, ":")
	if !ok {
		return "", fmt.Errorf("malformed secret ref %q", raw)
	}
	switch scheme {
	case "env":
		return r.Getenv(arg), nil
	case "file":
		b, err := r.ReadFile(arg)
		if err != nil {
			return "", fmt.Errorf("read secret file %q: %w", arg, err)
		}
		return strings.TrimSpace(string(b)), nil
	default:
		return "", fmt.Errorf("unknown secret scheme %q", scheme)
	}
}
