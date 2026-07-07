package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/daemon"
)

func TestServeCmd_Registered(t *testing.T) {
	require.NotNil(t, findCommand(NewRootCmd(), "serve"))
}

func TestMutatingVerbs_RefuseWhileDaemonRuns(t *testing.T) {
	repo := writeTempRepo(t, readOnlyConfig) // path to <dir>/gantry.yaml
	require.NoError(t, os.MkdirAll(filepath.Dir(lockPath(repo)), 0o755))
	l, err := daemon.Acquire(lockPath(repo))
	require.NoError(t, err)
	defer func() { _ = l.Release() }()

	for _, args := range [][]string{
		{"sync", "--env", "test"},
		{"deploy", "--env", "test"},
		{"promote", "--from", "test", "--to", "prod"},
		{"rollback", "--env", "prod"},
		{"switch", "--env", "front"},
	} {
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append(args, "--config", repo))
		err := cmd.Execute()
		require.ErrorContains(t, err, "reconciling this repo", "verb %v should refuse", args)
	}
}

func TestBuildServeMux_ServesMetricsAndHealthz(t *testing.T) {
	_, metricsHandler := daemon.NewPrometheusObserver("test")
	mux := buildServeMux(metricsHandler, doorbellMount{})

	for path, want := range map[string]string{"/healthz": "ok", "/metrics": "gantry_"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), want)
	}
}

func TestBuildServeMux_MountsDoorbellWhenProvided(t *testing.T) {
	h, _ := daemon.NewDoorbell("s3cret")
	mux := buildServeMux(nil, doorbellMount{Path: "/hooks/forge", Handler: h})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hooks/forge", nil)
	req.Header.Set("X-Gantry-Token", "s3cret")
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
}

// findCommand returns the named subcommand of root, or nil if absent.
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestResolveInterval(t *testing.T) {
	def := 60 * time.Second

	got, err := resolveInterval("", def)
	require.NoError(t, err)
	require.Equal(t, def, got) // empty flag ⇒ config default

	got, err = resolveInterval("1d", def)
	require.NoError(t, err)
	require.Equal(t, 24*time.Hour, got) // day suffix honored (C4)

	_, err = resolveInterval("30x", def)
	require.Error(t, err) // malformed ⇒ error, not silent fallback (C4)
}
