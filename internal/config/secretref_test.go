package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func testResolver() SecretResolver {
	return SecretResolver{
		Getenv: func(k string) string {
			if k == "TOK" {
				return "s3cret"
			}
			return ""
		},
		ReadFile: func(p string) ([]byte, error) {
			if p == "/run/secrets/key" {
				return []byte("FILEDATA\n"), nil
			}
			return nil, errors.New("not found")
		},
	}
}

func TestResolve_Env(t *testing.T) {
	v, err := testResolver().Resolve(SecretRef{Raw: "${env:TOK}"})
	require.NoError(t, err)
	require.Equal(t, "s3cret", v)
}

func TestResolve_File_Trimmed(t *testing.T) {
	v, err := testResolver().Resolve(SecretRef{Raw: "${file:/run/secrets/key}"})
	require.NoError(t, err)
	require.Equal(t, "FILEDATA", v)
}

func TestResolve_InlineSecretRejected(t *testing.T) {
	_, err := testResolver().Resolve(SecretRef{Raw: "literalpassword"})
	require.Error(t, err)
}

func TestResolve_Empty(t *testing.T) {
	v, err := testResolver().Resolve(SecretRef{})
	require.NoError(t, err)
	require.Equal(t, "", v)
}
