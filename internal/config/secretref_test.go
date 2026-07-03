package config

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func testResolver() SecretResolver {
	return SecretResolver{
		LookupEnv: func(k string) (string, bool) {
			switch k {
			case "TOK":
				return "s3cret", true
			case "EMPTY":
				return "", true // explicitly set to empty
			default:
				return "", false // unset
			}
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

func TestResolve_Env_UnsetIsError(t *testing.T) {
	_, err := testResolver().Resolve(SecretRef{Raw: "${env:MISSING}"})
	require.ErrorContains(t, err, "not set")
}

func TestResolve_Env_ExplicitEmptyIsAllowed(t *testing.T) {
	v, err := testResolver().Resolve(SecretRef{Raw: "${env:EMPTY}"})
	require.NoError(t, err)
	require.Equal(t, "", v)
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

func TestResolve_RegistryDispatch(t *testing.T) {
	Register("fake", func(_ SecretResolver, arg string) (string, error) { return "got:" + arg, nil })
	got, err := DefaultResolver().Resolve(SecretRef{Raw: "${fake:xyz}"})
	require.NoError(t, err)
	require.Equal(t, "got:xyz", got)
}

func TestResolve_UnknownSchemeStillErrors(t *testing.T) {
	_, err := DefaultResolver().Resolve(SecretRef{Raw: "${nope:x}"})
	require.ErrorContains(t, err, "unknown secret scheme")
}

func TestDefaultResolver_RunnerRunsCommand(t *testing.T) {
	out, err := DefaultResolver().Runner(context.Background(), nil, "go", "version")
	require.NoError(t, err)
	require.Contains(t, string(out), "go version")
}

func TestRunner_NonZeroExitCarriesStderr(t *testing.T) {
	// A command that fails should surface its stderr in the error.
	_, err := DefaultResolver().Runner(context.Background(), nil, "go", "definitely-not-a-subcommand")
	require.Error(t, err)
}

func TestResolveCmd(t *testing.T) {
	r := DefaultResolver()
	r.Runner = func(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
		require.Equal(t, "op", name)
		require.Equal(t, []string{"read", "op://vault/item/field"}, args)
		return []byte("s3cret\n"), nil
	}
	got, err := r.Resolve(SecretRef{Raw: "${cmd:op read op://vault/item/field}"})
	require.NoError(t, err)
	require.Equal(t, "s3cret", got) // trimmed
}

func TestResolveCmd_ErrorPropagates(t *testing.T) {
	r := DefaultResolver()
	r.Runner = func(context.Context, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("exit 1: denied")
	}
	_, err := r.Resolve(SecretRef{Raw: "${cmd:secret-tool get foo}"})
	require.ErrorContains(t, err, "denied")
}
