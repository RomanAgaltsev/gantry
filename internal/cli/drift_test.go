package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExitCode(t *testing.T) {
	require.Equal(t, 0, ExitCode(nil))
	require.Equal(t, 3, ExitCode(ErrDriftDetected))
	require.Equal(t, 3, ExitCode(errors.Join(ErrDriftDetected, errors.New("x")))) // wrapped still maps to 3
	require.Equal(t, 1, ExitCode(errors.New("boom")))
}

// TestDriftCommand drives `gantry drift --env test` end-to-end against a temp git repo
// and a stub forge. The stub serves a release published 10 days ago (past the 7d default
// threshold) under a newer ref than the committed pin, so the run must report drift and
// map to exit code 3. Re-pinning to that latest ref and re-running must report no drift.
func TestDriftCommand(t *testing.T) {
	t.Setenv("GANTRY_FORGE_TOKEN", "tok")

	// Stub forge: latest release reg/svc:v2 published 10 days ago (older than the 7d default).
	builtAt := time.Now().UTC().Add(-10 * 24 * time.Hour)
	srv := startDriftGitLab(t, builtAt)

	// initRepo (sync_test.go) seeds a committed pin file at the OLDER ref reg/svc:v1;
	// planCfg (sync_test.go) is a track-mode env "test" pinning component svc -> SVC_IMAGE.
	dir := initRepo(t)
	cfgPath := filepath.Join(dir, "gantry.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(planCfg(srv.URL)), 0o600))

	// ACT 1: latest (v2) differs from the pin (v1) and is stale -> drift.
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "drift", "--env", "test"})
	runErr := cmd.Execute()

	require.ErrorIs(t, runErr, ErrDriftDetected)
	require.Equal(t, 3, ExitCode(runErr))
	require.Contains(t, out.String(), "DRIFT test/")

	// Re-pin to the latest ref (incl. digest) and commit; gantry reads HEAD, not the worktree.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.versions.test"),
		[]byte("SVC_IMAGE=reg/svc:v2@sha256:d\n"), 0o600))
	gitCommit(t, dir, "pin to latest")

	// ACT 2: pin now equals latest -> no drift, exit 0.
	cmd2 := NewRootCmd()
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetErr(&out2)
	cmd2.SetArgs([]string{"--config", cfgPath, "drift", "--env", "test"})
	runErr2 := cmd2.Execute()

	require.NoError(t, runErr2)
	require.Contains(t, out2.String(), "no drift")
}

// startDriftGitLab stubs the GitLab Releases API with a single release whose metadata's
// built_at is caller-controlled, so the test can place it before/after the drift threshold.
func startDriftGitLab(t *testing.T, builtAt time.Time) *httptest.Server {
	t.Helper()
	body := "<!-- gantry-release-metadata:v1:start -->\n```json\n" +
		`{"schema_version":"1","component":"svc","semver_version":"v2",` +
		`"image_repository":"reg/svc","image_tag":"v2","image_digest":"sha256:d",` +
		`"commit_sha":"c","built_at":"` + builtAt.Format(time.RFC3339) + `","changelog_section":"x"}` +
		"\n```\n<!-- gantry-release-metadata:v1:end -->"
	mb, _ := json.Marshal(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2","description":` + string(mb) + `}]`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// gitCommit stages and commits the worktree of an already-initialized repo (initRepo
// sets the author identity), so a freshly written pin file becomes the HEAD gantry reads.
func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", msg}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))
	}
}
