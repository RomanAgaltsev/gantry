package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// validCfg references only env-backed secrets, so a test can make them all resolve via
// t.Setenv. The ambient secrets.vault.* refs are deliberately absent so validate does not
// require VAULT_ADDR/VAULT_TOKEN to be set.
const validCfg = `
forge: { kind: gitlab, base_url: https://x, token: "${env:V_TOK}" }
connections:
  h:
    address: 10.0.0.1
    ssh:
      user: deploy
      key: "${env:V_KEY}"
      known_hosts: "${env:V_KH}"
components:
  - { id: svc, project: g/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor:
      kind: compose-over-ssh
      connection: h
      project_dir: /opt/app
      compose_files: [compose.yaml]
      env_file: .env.versions.test
`

func TestValidate_OK(t *testing.T) {
	t.Setenv("V_TOK", "t")
	t.Setenv("V_KEY", "k")
	t.Setenv("V_KH", "kh")
	path := writeTempRepo(t, validCfg)

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"validate", "--config", path})
	require.NoError(t, root.Execute())
	require.Contains(t, out.String(), "config valid")
}

func TestValidate_UnsetSecretFails(t *testing.T) {
	t.Setenv("V_KEY", "k")
	t.Setenv("V_KH", "kh") // V_TOK deliberately left unset
	path := writeTempRepo(t, validCfg)

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"validate", "--config", path})
	err := root.Execute()
	require.Error(t, err) // unset ${env:V_TOK} ⇒ validation error
	require.Contains(t, err.Error(), "V_TOK")
}

// TestValidate_DoesNotRequireVaultEnv ensures a config without any ${vault:…} ref validates
// even when VAULT_ADDR/VAULT_TOKEN are unset (the ambient vault refs are best-effort, excluded
// from the resolve-all check).
func TestValidate_DoesNotRequireVaultEnv(t *testing.T) {
	t.Setenv("V_TOK", "t")
	t.Setenv("V_KEY", "k")
	t.Setenv("V_KH", "kh")
	// VAULT_ADDR / VAULT_TOKEN deliberately unset.
	path := writeTempRepo(t, validCfg)

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"validate", "--config", path})
	require.NoError(t, root.Execute())
}
