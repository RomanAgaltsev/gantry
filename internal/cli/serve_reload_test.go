package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
)

// serveReloadCfg resolves cleanly: an env-backed forge token plus read-only SSH/registry refs
// that serveDeps never touches for the no-remote, no-exec path it builds at startup.
const serveReloadCfg = `
forge:
  kind: gitlab
  base_url: https://gitlab.example.com
  token: ${env:GANTRY_SERVE_TOK}
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

func TestReloadDeps_ValidConfigSwaps(t *testing.T) {
	t.Setenv("GANTRY_SERVE_TOK", "tok")
	path := writeTempRepo(t, serveReloadCfg)

	deps, cfg, err := reloadDeps(t.Context(), path, config.DefaultResolver())
	require.NoError(t, err)
	require.NotNil(t, deps)
	require.NotNil(t, deps.Engine)
	require.Len(t, cfg.Environments, 1)
}

func TestReloadDeps_InvalidConfigErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gantry.yaml")
	// Malformed YAML: a mapping that config.Load rejects.
	require.NoError(t, os.WriteFile(path, []byte("\tforge: { kind: gitlab\n"), 0o600))

	_, _, err := reloadDeps(t.Context(), path, config.DefaultResolver())
	require.Error(t, err) // parse failure ⇒ error, so the caller keeps the previous deps
}
