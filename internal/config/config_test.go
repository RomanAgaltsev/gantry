package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestLoad_VaultDefaults(t *testing.T) {
	c, err := Load(writeCfg(t, goodCfg))
	require.NoError(t, err)
	// Vault address/token default to the standard env vars when the secrets.vault block is unset.
	require.Equal(t, "${env:VAULT_ADDR}", c.Secrets.Vault.Address.Raw)
	require.Equal(t, "${env:VAULT_TOKEN}", c.Secrets.Vault.Token.Raw)
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

const blueGreenCfg = `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: front
    source: { track: latest }
    pin_file: .env.versions.front
    executor:
      kind: blue-green
      connection: h
      slots:
        blue:  { project_dir: /opt/front-blue,  compose_files: [compose.yaml] }
        green: { project_dir: /opt/front-green, compose_files: [compose.yaml] }
      pointer:
        link: /etc/nginx/conf.d/front.conf
        blue: /etc/nginx/conf.d/upstream-blue.conf
        green: /etc/nginx/conf.d/upstream-green.conf
        reload: "nginx -s reload"
`

func TestLoad_BlueGreen(t *testing.T) {
	c, err := Load(writeCfg(t, blueGreenCfg))
	require.NoError(t, err)
	env, _ := c.Environment("front")
	require.Equal(t, "/opt/front-blue", env.Executor.Slots["blue"].ProjectDir)
	require.Equal(t, "nginx -s reload", env.Executor.Pointer.Reload)
}

func TestLoad_BlueGreenInvalid(t *testing.T) {
	// missing the green slot
	bad := `
forge: { kind: gitlab, base_url: https://x, token: "${env:T}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/k}" } }
components:
  - { id: svc, project: grp/svc, pin_key: SVC_IMAGE }
environments:
  - name: front
    source: { track: latest }
    pin_file: .env.versions.front
    executor:
      kind: blue-green
      connection: h
      slots: { blue: { project_dir: /opt/front-blue } }
      pointer: { link: /l, blue: /b, green: /g, reload: "r" }
`
	_, err := Load(writeCfg(t, bad))
	require.Error(t, err)
	require.Contains(t, err.Error(), "green")
}

func TestLoad_VerifyOnFailure_RollbackValid(t *testing.T) {
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
    verify:
      - { kind: http, url: https://app/healthz }
    verify_on_failure: rollback
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	c, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
	env, _ := c.Environment("test")
	require.True(t, env.RollbackOnVerifyFailure())
}

func TestLoad_VerifyOnFailure_DefaultHold(t *testing.T) {
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
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	c, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
	env, _ := c.Environment("test")
	require.False(t, env.RollbackOnVerifyFailure())
}

func TestLoad_VerifyOnFailure_InvalidValue(t *testing.T) {
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
    verify:
      - { kind: http, url: https://app/healthz }
    verify_on_failure: maybe
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, cfg))
	require.ErrorContains(t, err, "verify_on_failure")
}

func TestLoad_VerifyOnFailure_RollbackRequiresProbes(t *testing.T) {
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
    verify_on_failure: rollback
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
`
	_, err := Load(writeCfg(t, cfg))
	require.ErrorContains(t, err, "requires at least one verify probe")
}

func TestLoad_Notifications_OK(t *testing.T) {
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
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
notifications:
  - kind: webhook
    url: "${env:HOOK}"
    chat_id: "${env:CHAT}"
    events: [deployed, verify_failed]
  - kind: email
    smtp: { host: smtp.x, port: 587, username: ops, password: "${file:/s}" }
    from: gantry@x
    to: [ops@x]
`
	c, err := Load(writeCfg(t, cfg))
	require.NoError(t, err)
	require.Len(t, c.Notifications, 2)
	require.Equal(t, "webhook", c.Notifications[0].Kind)
	require.Equal(t, []string{"deployed", "verify_failed"}, c.Notifications[0].Events)
	require.Equal(t, 587, c.Notifications[1].SMTP.Port)
}

func TestLoad_Notifications_Invalid(t *testing.T) {
	base := func(block string) string {
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
    executor: { kind: compose-over-ssh, connection: h, project_dir: /o }
notifications:
` + block
	}
	for _, block := range []string{
		"  - { kind: sms, url: x }",                     // unknown kind
		"  - { kind: webhook }",                         // webhook without url
		"  - { kind: email, from: g@x, to: [o@x] }",     // email without smtp.host
		"  - { kind: webhook, url: x, events: [boom] }", // unknown event
	} {
		_, err := Load(writeCfg(t, base(block)))
		require.Error(t, err, block)
	}
}

func TestLoad_ComposePSAllowedOnSymlinkRelease(t *testing.T) {
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
    verify:
      - { kind: compose-ps }
    executor: { kind: symlink-release, connection: h, project_dir: /opt/app, compose_files: [compose.yaml] }
`
	_, err := Load(writeCfg(t, cfg))
	require.NoError(t, err) // kind-aware ComposeTarget resolves the project at verify time
}

// withDaemon appends a daemon: block (built from a snippet) to the valid base config.
func withDaemon(snippet string) string {
	return goodCfg + "daemon:\n  " + snippet + "\n"
}

// loadYAMLErr writes body to a temp config and loads it, returning any error.
func loadYAMLErr(t *testing.T, body string) (*Config, error) {
	t.Helper()
	return Load(writeCfg(t, body))
}

func TestLoad_DaemonDefaults(t *testing.T) {
	cfg, err := Load(writeCfg(t, goodCfg)) // no daemon: block → all fields default
	require.NoError(t, err)
	require.Equal(t, 60*time.Second, cfg.Daemon.Interval.Duration())
	require.Equal(t, "127.0.0.1:9713", cfg.Daemon.Listen)
	require.False(t, cfg.Daemon.Doorbell.Enabled)
	require.Equal(t, "/hooks/forge", cfg.Daemon.Doorbell.Path)
}

func TestLoad_DaemonIntervalTooSmall(t *testing.T) {
	// A non-zero sub-1s interval is rejected; 0 would be defaulted to 60s before validation.
	_, err := loadYAMLErr(t, withDaemon(`interval: 500ms`))
	require.ErrorContains(t, err, "daemon.interval")
}

func TestLoad_DefaultsReconcileTimeout(t *testing.T) {
	cfg, err := Load(writeCfg(t, goodCfg)) // no daemon: block ⇒ all fields default
	require.NoError(t, err)
	require.Equal(t, 5*time.Minute, cfg.Daemon.ReconcileTimeout.Duration())
}

func TestLoad_ReconcileTimeoutTooSmall(t *testing.T) {
	_, err := loadYAMLErr(t, withDaemon(`reconcile_timeout: 500ms`))
	require.ErrorContains(t, err, "daemon.reconcile_timeout")
}

func TestLoad_DoorbellEnabledRequiresSecret(t *testing.T) {
	_, err := loadYAMLErr(t, withDaemon("doorbell:\n    enabled: true"))
	require.ErrorContains(t, err, "doorbell")
}
