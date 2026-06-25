package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	return dir
}

func sig() object.Signature {
	return object.Signature{Name: "gantry", Email: "gantry@local"}
}

func TestGitLedger_RecordThenQuery(t *testing.T) {
	dir := newRepo(t)
	l, err := NewGitLedger(dir, sig())
	require.NoError(t, err)

	require.NoError(t, l.Record(Entry{
		Environment: "test", PinCommit: "aaa", Result: "ok", Healthy: "unknown",
		ImageSet: map[string]string{"SVC_IMAGE": "reg/svc:v1"}, DeployedAt: time.Now(), By: "sync",
	}))

	// the ledger file exists and was committed
	_, err = os.Stat(filepath.Join(dir, ".gantry", "deploys.jsonl"))
	require.NoError(t, err)
	repo, _ := git.PlainOpen(dir)
	_, err = repo.Head() // a commit exists
	require.NoError(t, err)

	got, ok, err := l.Lookup("test", "aaa")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "ok", got.Result)
	require.Equal(t, "reg/svc:v1", got.ImageSet["SVC_IMAGE"])

	green, err := l.LatestGreen("test")
	require.NoError(t, err)
	require.Equal(t, "aaa", green.PinCommit)
}

func TestGitLedger_LatestGreen_None(t *testing.T) {
	dir := newRepo(t)
	l, _ := NewGitLedger(dir, sig())
	require.NoError(t, l.Record(Entry{Environment: "test", PinCommit: "aaa", Result: "failed", By: "sync"}))
	_, err := l.LatestGreen("test")
	require.ErrorIs(t, err, ErrNoGreen)
}
