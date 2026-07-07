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
		Environment: "test", PinCommit: "aaa", Result: ResultOK, Healthy: HealthUnknown,
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
	require.Equal(t, ResultOK, got.Result)
	require.Equal(t, "reg/svc:v1", got.ImageSet["SVC_IMAGE"])

	green, err := l.LatestGreen("test")
	require.NoError(t, err)
	require.Equal(t, "aaa", green.PinCommit)
}

func TestGitLedger_Record_RefusesPreStagedFile(t *testing.T) {
	dir := newRepo(t)
	l, err := NewGitLedger(dir, sig())
	require.NoError(t, err)
	require.NoError(t, l.Record(Entry{Environment: "test", PinCommit: "aaa", Result: ResultOK, By: "sync"}))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.txt"), []byte("x"), 0o600))
	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("user.txt")
	require.NoError(t, err)

	err = l.Record(Entry{Environment: "test", PinCommit: "bbb", Result: ResultOK, By: "sync"})
	require.ErrorContains(t, err, "user.txt")
}

func TestGitLedger_LatestGreen_None(t *testing.T) {
	dir := newRepo(t)
	l, _ := NewGitLedger(dir, sig())
	require.NoError(t, l.Record(Entry{Environment: "test", PinCommit: "aaa", Result: ResultFailed, By: "sync"}))
	_, err := l.LatestGreen("test")
	require.ErrorIs(t, err, ErrNoGreen)
}

func TestLatestHealthy(t *testing.T) {
	entries := []Entry{
		{Environment: "test", PinCommit: "a", Result: ResultOK, Healthy: HealthTrue},
		{Environment: "test", PinCommit: "b", Result: ResultOK, Healthy: HealthUnknown},
		{Environment: "test", PinCommit: "c", Result: ResultFailed, Healthy: HealthFalse},
	}
	e, ok := latestHealthy(entries, "test")
	require.True(t, ok)
	require.Equal(t, "a", e.PinCommit) // newest ok+healthy, skipping unknown and failed

	_, ok = latestHealthy([]Entry{{Environment: "test", Result: ResultOK, Healthy: HealthUnknown}}, "test")
	require.False(t, ok)
}
