package verify

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeRunner returns canned output/error and records the command it was asked to run.
type fakeRunner struct {
	out    string
	err    error
	gotCmd string
}

func (r *fakeRunner) Run(_ context.Context, cmd string, _ []byte) (string, error) {
	r.gotCmd = cmd
	return r.out, r.err
}

func TestCommandVerifier(t *testing.T) {
	r := &fakeRunner{}
	require.NoError(t, CommandVerifier{Runner: r, Command: "true"}.Verify(context.Background()))
	require.Equal(t, "true", r.gotCmd)

	bad := &fakeRunner{err: errors.New("exit 1")}
	require.Error(t, CommandVerifier{Runner: bad, Command: "false"}.Verify(context.Background()))
}

func TestComposePSVerifier(t *testing.T) {
	v := ComposePSVerifier{ProjectDir: "/opt/app", ComposeFiles: []string{"compose.yaml"}, EnvFile: ".env"}

	t.Run("all running", func(t *testing.T) {
		r := &fakeRunner{out: `{"Service":"api","State":"running","Health":"healthy"}` + "\n" +
			`{"Service":"web","State":"running","Health":""}`}
		v.Runner = r
		require.NoError(t, v.Verify(context.Background()))
		require.Contains(t, r.gotCmd, "docker compose")
		require.Contains(t, r.gotCmd, "ps --format json")
	})

	t.Run("array form", func(t *testing.T) {
		v.Runner = &fakeRunner{out: `[{"Service":"api","State":"running","Health":"healthy"}]`}
		require.NoError(t, v.Verify(context.Background()))
	})

	t.Run("not running fails", func(t *testing.T) {
		v.Runner = &fakeRunner{out: `{"Service":"api","State":"exited","Health":""}`}
		err := v.Verify(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "api")
	})

	t.Run("unhealthy fails", func(t *testing.T) {
		v.Runner = &fakeRunner{out: `{"Service":"api","State":"running","Health":"unhealthy"}`}
		require.Error(t, v.Verify(context.Background()))
	})

	t.Run("no services fails", func(t *testing.T) {
		v.Runner = &fakeRunner{out: ""}
		require.Error(t, v.Verify(context.Background()))
	})

	t.Run("malformed json fails", func(t *testing.T) {
		v.Runner = &fakeRunner{out: "not json"}
		require.Error(t, v.Verify(context.Background()))
	})
}
