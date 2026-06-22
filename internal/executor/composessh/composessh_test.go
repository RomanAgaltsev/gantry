package composessh

import (
	"context"
	"strings"
	"testing"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/pin"
	"github.com/stretchr/testify/require"
)

type fakeRunner struct {
	cmds   []string
	stdins [][]byte
}

func (f *fakeRunner) Run(_ context.Context, cmd string, stdin []byte) (string, error) {
	f.cmds = append(f.cmds, cmd)
	f.stdins = append(f.stdins, stdin)
	return "", nil
}

func TestDeploy_WritesEnvThenPullsAndUps(t *testing.T) {
	fr := &fakeRunner{}
	ex := &Executor{
		Runner:       fr,
		ProjectDir:   "/opt/app",
		ComposeFiles: []string{"compose.yaml"},
		EnvFile:      ".env.versions.test",
	}
	res, err := ex.Deploy(context.Background(), executor.Plan{
		Env:     "test",
		PinFile: ".env.versions.test",
		Pins:    pin.Set{"A_IMAGE": "reg/a:v1"},
	})
	require.NoError(t, err)
	require.True(t, res.Changed)

	require.Len(t, fr.cmds, 3)
	// 1) write env file at the project dir
	require.Contains(t, fr.cmds[0], "/opt/app/.env.versions.test")
	require.Equal(t, "A_IMAGE=reg/a:v1\n", string(fr.stdins[0]))
	// 2) pull, 3) up -d, both scoped to project dir + compose file + env file
	require.Contains(t, fr.cmds[1], "cd /opt/app")
	require.Contains(t, fr.cmds[1], "-f compose.yaml")
	require.Contains(t, fr.cmds[1], "--env-file .env.versions.test")
	require.True(t, strings.Contains(fr.cmds[1], "pull"))
	require.True(t, strings.Contains(fr.cmds[2], "up -d"))
}

func TestRegistryHostOf(t *testing.T) {
	cases := map[string]string{
		"gitlab.rarus.ru:5050/g/s:v1": "gitlab.rarus.ru:5050",
		"ghcr.io/org/img:v2":          "ghcr.io",
		"postgres:16.4":               "docker.io",
		"myorg/myimage:tag":           "docker.io",
		"localhost:5000/x:1":          "localhost:5000",
	}
	for ref, want := range cases {
		require.Equal(t, want, registryHostOf(ref), ref)
	}
}

func TestDeploy_LogsInOnlyMatchingRegistriesBeforePull(t *testing.T) {
	fr := &fakeRunner{}
	ex := &Executor{
		Runner:       fr,
		ProjectDir:   "/opt/app",
		ComposeFiles: []string{"compose.yaml"},
		EnvFile:      ".env.versions.test",
		Logins: []RegistryLogin{
			{Registry: "gitlab.rarus.ru:5050", Username: "u", Password: "p"},
			{Registry: "ghcr.io", Username: "g", Password: "q"}, // not referenced -> skipped
		},
	}
	_, err := ex.Deploy(context.Background(), executor.Plan{
		Env: "test", PinFile: ".env.versions.test",
		Pins: pin.Set{"SVC_IMAGE": "gitlab.rarus.ru:5050/g/s:v1"},
	})
	require.NoError(t, err)

	// cmds: [write-env, login gitlab, pull, up] — ghcr login absent
	require.Len(t, fr.cmds, 4)
	require.Contains(t, fr.cmds[1], "docker login gitlab.rarus.ru:5050 -u u --password-stdin")
	require.Equal(t, "p", string(fr.stdins[1])) // password via stdin
	require.Contains(t, fr.cmds[2], "pull")
	for _, c := range fr.cmds {
		require.NotContains(t, c, "ghcr.io")
	}
}
