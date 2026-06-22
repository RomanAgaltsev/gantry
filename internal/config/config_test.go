package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gantry.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

const goodCfg = `
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_FORGE_TOKEN}
connections:
  test-host:
    address: 10.0.0.1
    ssh: { user: deploy, key: ${file:/run/secrets/key} }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor:
      kind: compose-over-ssh
      connection: test-host
      project_dir: /opt/app
      compose_files: [compose.yaml]
      env_file: .env.versions.test
`

func TestLoad_OK_AndDefaults(t *testing.T) {
	c, err := Load(writeCfg(t, goodCfg))
	require.NoError(t, err)
	require.Equal(t, "gantry-release-metadata", c.Forge.MetadataMarker)
	env, ok := c.Environment("test")
	require.True(t, ok)
	require.Equal(t, "latest", env.Source.Track)
}

func TestLoad_DanglingConnection(t *testing.T) {
	bad := goodCfg + `
  - name: prod
    source: { promote_from: test }
    pin_file: .env.versions.prod
    executor: { kind: compose-over-ssh, connection: nope, project_dir: /opt/app }
`
	_, err := Load(writeCfg(t, bad))
	require.ErrorContains(t, err, "connection")
}

func TestLoad_NoSource(t *testing.T) {
	bad := `
forge: { kind: gitlab, base_url: https://x, token: ${env:T} }
components: [{ id: svc, project: g/s, pin_key: SVC_IMAGE }]
environments:
  - name: test
    source: {}
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
connections: { h: { address: 1.1.1.1 } }
`
	_, err := Load(writeCfg(t, bad))
	require.ErrorContains(t, err, "source")
}
