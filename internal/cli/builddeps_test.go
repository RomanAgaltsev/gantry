package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/require"
)

// readOnlyConfig points its forge token, SSH creds, and registry creds at files that do
// not exist. A read-only command must not try to resolve any of them.
const readOnlyConfig = `
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${file:/does/not/exist}
connections:
  app-host:
    address: 10.0.0.1
    ssh:
      user: deploy
      key: ${file:/does/not/exist}
      known_hosts: ${file:/does/not/exist}
registries:
  reg.example.com:
    user: ${file:/does/not/exist}
    password: ${file:/does/not/exist}
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

func writeTempRepo(t *testing.T, configYAML string) string {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	path := filepath.Join(dir, "gantry.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configYAML), 0o600))
	return path
}

func TestHistoryCmd_DoesNotResolveForgeOrRegistrySecrets(t *testing.T) {
	path := writeTempRepo(t, readOnlyConfig)

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"history", "--env", "test", "--config", path})

	require.NoError(t, cmd.Execute()) // must not resolve forge/ssh/registry secrets
	require.Contains(t, out.String(), "no deploy history")
}
