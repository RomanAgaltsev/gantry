package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// This test exercises `plan` end-to-end against a fake GitLab server + temp git repo.
// It is wired in Step 3 once buildDeps is factored to allow a custom forge base URL via config.
func TestPlanCommand_ReportsPendingChange(t *testing.T) {
	t.Setenv("GANTRY_FORGE_TOKEN", "tok")

	srv := startFakeGitLab(t) // helper defined below; serves one release reg/svc:v2
	dir := initRepo(t)        // helper: git init temp dir with a pin file at reg/svc:v1

	cfgPath := filepath.Join(dir, "gantry.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(planCfg(srv.URL)), 0o600))

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "plan", "--env", "test"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "SVC_IMAGE")
	require.Contains(t, out.String(), "reg/svc:v2")
}

func startFakeGitLab(t *testing.T) *httptest.Server {
	t.Helper()
	body := "<!-- gantry-release-metadata:v1:start -->\n```json\n" +
		`{"schema_version":"1","component":"svc","semver_version":"v2",` +
		`"image_repository":"reg/svc","image_tag":"v2","image_digest":"sha256:d",` +
		`"commit_sha":"c","built_at":"2026-06-18T09:00:00Z","changelog_section":"x"}` +
		"\n```\n<!-- gantry-release-metadata:v1:end -->"
	mb, _ := jsonMarshal(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2","description":` + string(mb) + `}]`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		require.NoError(t, c.Run())
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.versions.test"), []byte("SVC_IMAGE=reg/svc:v1\n"), 0o644))
	return dir
}

func planCfg(forgeURL string) string {
	return `
forge: { kind: gitlab, base_url: ` + forgeURL + `, token: "${env:GANTRY_FORGE_TOKEN}" }
connections: { test-host: { address: 127.0.0.1, ssh: { user: x, key: "${env:GANTRY_FORGE_TOKEN}", known_hosts: "${env:GANTRY_FORGE_TOKEN}" } } }
components: [{ id: svc, project: g/svc, pin_key: SVC_IMAGE }]
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: test-host, project_dir: /opt/app, compose_files: [compose.yaml], env_file: .env.versions.test }
`
}
