package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type statusFakeForge struct{}

func (statusFakeForge) LatestRelease(_ context.Context, c forge.Component) (forge.Release, error) {
	return forge.Release{ImageRepository: "reg/" + c.ID, ImageTag: "v9"}, nil
}

func TestComponentStatusLine_Explicit(t *testing.T) {
	line := componentStatusLine(context.Background(),
		config.Component{PinKey: "POSTGRES_IMAGE", Source: config.ComponentSource{Pin: "explicit"}},
		pin.Set{"POSTGRES_IMAGE": "postgres:16.4"}, statusFakeForge{})
	require.Contains(t, line, "postgres:16.4")
	require.Contains(t, line, "latest=(untracked)")
}

func TestComponentStatusLine_ForgeRelease(t *testing.T) {
	line := componentStatusLine(context.Background(),
		config.Component{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
		pin.Set{"SVC_IMAGE": "reg/svc:v1"}, statusFakeForge{})
	require.Contains(t, line, "latest=reg/svc:v9")
}

type statusErrForge struct{}

func (statusErrForge) LatestRelease(context.Context, forge.Component) (forge.Release, error) {
	return forge.Release{}, errors.New("forge down")
}

func TestComponentStatusLine_ForgeErrorDegrades(t *testing.T) {
	line := componentStatusLine(context.Background(),
		config.Component{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
		pin.Set{"SVC_IMAGE": "reg/svc:v1"}, statusErrForge{})
	require.Contains(t, line, "latest=(error)")
}

func TestBuildDeps_EmptyEnvForMatrix(t *testing.T) {
	// The forge token must be a ${...} ref — config.Resolve rejects inline literals.
	t.Setenv("GANTRY_TEST_TOK", "tok")
	const cfgYAML = `
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_TEST_TOK}
connections:
  app-host:
    address: 10.0.0.1
    ssh:
      user: deploy
      key: ${file:/does/not/exist}
      known_hosts: ${file:/does/not/exist}
components:
  - { id: svc, project: g/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor:
      kind: compose-over-ssh
      connection: app-host
      project_dir: /opt/app
      compose_files: [compose.yaml]
      env_file: .env.versions.test
`
	path := writeTempRepo(t, cfgYAML)

	// Parse the persistent --config flag, then call buildDeps with an empty env
	// name (the matrix path). It must succeed and resolve no executor.
	root := NewRootCmd()
	require.NoError(t, root.ParseFlags([]string{"--config", path}))

	d, err := buildDeps(root, "", true, false)
	require.NoError(t, err)
	require.Equal(t, "", d.env)
	require.NotNil(t, d.engine.Forge)
	require.Nil(t, d.exec) // read-only: no executor
}

func TestStatusCmd_AllAndEnvMutuallyExclusive(t *testing.T) {
	path := writeTempRepo(t, readOnlyConfig)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "--all", "--env", "test", "--config", path})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestStatusCmd_RequiresEnvOrAll(t *testing.T) {
	path := writeTempRepo(t, readOnlyConfig)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "--config", path})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "one of --env or --all")
}

func TestStatusCmd_WatchJSONIncompatible(t *testing.T) {
	path := writeTempRepo(t, readOnlyConfig)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "--watch", "-o", "json", "--config", path})

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "incompatible")
}

// TestStatusCmd_WatchRendersUntilCancelled drives one --watch refresh and cancels the
// command context so the loop exits cleanly. The output must contain the matrix render.
func TestStatusCmd_WatchRendersUntilCancelled(t *testing.T) {
	prev := newForgeFunc
	newForgeFunc = func(config.ForgeConfig, string) (forge.Forge, error) { return statusFakeForge{}, nil }
	t.Cleanup(func() { newForgeFunc = prev })

	t.Setenv("GANTRY_TEST_TOK", "tok")
	const cfgYAML = `
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_TEST_TOK}
connections:
  app-host:
    address: 10.0.0.1
    ssh:
      user: deploy
      key: ${file:/does/not/exist}
      known_hosts: ${file:/does/not/exist}
components:
  - { id: svc, project: g/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor:
      kind: compose-over-ssh
      connection: app-host
      project_dir: /opt/app
      compose_files: [compose.yaml]
      env_file: .env.versions.test
`

	path := writeTempRepo(t, cfgYAML)
	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"status", "--watch", "--interval", "1ms", "--config", path})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cmd.Execute()
	}()
	// Let one refresh render, then cancel so the loop returns.
	require.Eventually(t, func() bool { return out.Len() > 0 }, time.Second, time.Millisecond)
	cancel()
	<-done

	require.Contains(t, out.String(), clearScreen)
	require.Contains(t, out.String(), "reg/svc:v9") // one matrix render happened
}
