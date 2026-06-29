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
    ssh: { user: deploy, key: "${file:/run/secrets/key}" }
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
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
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

const explicitCfg = `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections: { h: { address: 1.1.1.1, ssh: { user: x, key: "${env:T}" } } }
components:
  - { id: svc, project: g/svc, pin_key: SVC_IMAGE }
  - id: postgres
    pin_key: POSTGRES_IMAGE
    source: { pin: explicit }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`

func TestLoad_ExplicitComponent(t *testing.T) {
	c, err := Load(writeCfg(t, explicitCfg))
	require.NoError(t, err)
	require.True(t, c.Components[0].IsForgeRelease()) // svc defaulted
	require.False(t, c.Components[0].IsExplicit())
	require.True(t, c.Components[1].IsExplicit()) // postgres
}

func TestLoad_ExplicitWithProjectRejected(t *testing.T) {
	bad := `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections: { h: { address: 1.1.1.1 } }
components:
  - id: postgres
    project: oops
    pin_key: POSTGRES_IMAGE
    source: { pin: explicit }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, bad))
	require.ErrorContains(t, err, "project")
}

func TestLoad_ComposeConnectionRequiresSSH(t *testing.T) {
	bad := `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections: { h: { address: 1.1.1.1 } }
components: [{ id: svc, project: g/svc, pin_key: SVC_IMAGE }]
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, bad))
	require.ErrorContains(t, err, "ssh")
}

func TestLoad_GitIdentityDefaults(t *testing.T) {
	c, err := Load(writeCfg(t, goodCfg))
	require.NoError(t, err)
	require.Equal(t, "gantry", c.Git.AuthorName)
	require.Equal(t, "gantry@local", c.Git.AuthorEmail)
}

func TestLoad_ForgeReleaseRequiresProject(t *testing.T) {
	bad := `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections: { h: { address: 1.1.1.1 } }
components:
  - { id: svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, bad))
	require.ErrorContains(t, err, "project")
}

func TestLoad_GitHubDefaultsBaseURL(t *testing.T) {
	const cfg = `
forge:
  kind: github
  token: ${env:GH}
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: octo/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	c, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
	require.Equal(t, "https://api.github.com", c.Forge.BaseURL)
}

func TestLoad_GitHubEnterpriseBaseURLPreserved(t *testing.T) {
	const cfg = `
forge:
  kind: github
  base_url: https://ghe.example.com/api/v3
  token: ${env:GH}
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: octo/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	c, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
	require.Equal(t, "https://ghe.example.com/api/v3", c.Forge.BaseURL)
}

func TestLoad_UnknownForgeKind(t *testing.T) {
	const cfg = `
forge: { kind: bitbucket, base_url: https://x, token: "${env:T}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: octo/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, cfg))
	require.Error(t, err)
	require.Contains(t, err.Error(), "gitlab")
	require.Contains(t, err.Error(), "github")
}

func TestLoad_VerifyProbes(t *testing.T) {
	const cfg = `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
promote: { require_healthy: true }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    verify:
      - { kind: http, url: https://app/healthz }
      - { kind: compose-ps }
      - { kind: command, command: "test -f /opt/.ready" }
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	c, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
	require.True(t, c.Promote.RequireHealthy)
	env, _ := c.Environment("test")
	require.Len(t, env.Verify, 3)
	require.Equal(t, 200, env.Verify[0].ExpectStatus) // defaulted
}

func TestLoad_VerifyInvalid(t *testing.T) {
	base := func(probe string) string {
		return `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    verify: [ ` + probe + ` ]
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	}
	for _, probe := range []string{
		`{ kind: ftp }`,     // unknown kind
		`{ kind: http }`,    // http without url
		`{ kind: command }`, // command without command
	} {
		_, err := Load(writeCfg(t, base(probe)))
		require.Error(t, err, probe)
	}
}

func TestLoad_SymlinkReleaseExecutor(t *testing.T) {
	const cfg = `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: prod
    source: { promote_from: prod }
    pin_file: .env.versions.prod
    executor: { kind: symlink-release, connection: h, project_dir: /opt/app, compose_files: [compose.yaml] }
`
	_, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
}

func TestLoad_UnknownExecutorKind(t *testing.T) {
	const cfg = `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: nomad, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, cfg))
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink-release")
}
