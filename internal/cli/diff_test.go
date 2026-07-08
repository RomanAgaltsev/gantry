package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

const twoEnvCfg = `
forge: { kind: gitlab, base_url: https://x, token: "${file:/does/not/exist}" }
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
  - name: prod
    source: { promote_from: test }
    pin_file: .env.versions.prod
    executor:
      kind: compose-over-ssh
      connection: app-host
      project_dir: /opt/app
      compose_files: [compose.yaml]
      env_file: .env.versions.prod
`

func seedPin(t *testing.T, configPath, pinFile string, set pin.Set, msg string) {
	t.Helper()
	store, err := engine.NewGitStore(filepath.Dir(configPath), object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)
	_, err = store.WriteAndCommit(t.Context(), pinFile, set, msg)
	require.NoError(t, err)
}

func TestDiff_ShowsDifferences(t *testing.T) {
	path := writeTempRepo(t, twoEnvCfg)
	seedPin(t, path, ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:v1"}, "test")
	seedPin(t, path, ".env.versions.prod", pin.Set{"SVC_IMAGE": "reg/svc:v2"}, "prod")

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"diff", "--env", "test", "--to", "prod", "--config", path})
	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), "SVC_IMAGE")
	require.Contains(t, out.String(), "reg/svc:v1")
	require.Contains(t, out.String(), "reg/svc:v2")
}

func TestDiff_IdenticalEnvironments(t *testing.T) {
	path := writeTempRepo(t, twoEnvCfg)
	seedPin(t, path, ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:v1"}, "test")
	seedPin(t, path, ".env.versions.prod", pin.Set{"SVC_IMAGE": "reg/svc:v1"}, "prod")

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"diff", "--env", "test", "--to", "prod", "--config", path})
	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), "identical")
}

func TestDiff_JSONOutput(t *testing.T) {
	path := writeTempRepo(t, twoEnvCfg)
	seedPin(t, path, ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:v1"}, "test")
	seedPin(t, path, ".env.versions.prod", pin.Set{"SVC_IMAGE": "reg/svc:v2"}, "prod")

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"diff", "--env", "test", "--to", "prod", "-o", "json", "--config", path})
	require.NoError(t, root.Execute())

	var rows []diffRow
	require.NoError(t, json.Unmarshal(out.Bytes(), &rows))
	require.Len(t, rows, 1)
	require.Equal(t, "SVC_IMAGE", rows[0].Key)
	require.Equal(t, "reg/svc:v1", rows[0].A)
	require.Equal(t, "reg/svc:v2", rows[0].B)
}
