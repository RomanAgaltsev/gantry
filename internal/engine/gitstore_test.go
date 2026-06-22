package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func newRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	return dir, repo
}

func TestGitStore_CommitStampsCurrentTime(t *testing.T) {
	dir, repo := newRepo(t)
	store, err := NewGitStore(dir, object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)

	before := time.Now().Add(-time.Second)
	require.NoError(t, store.WriteAndCommit(".env.versions.test", pin.Set{"A": "reg/a:v1"}, "msg"))
	after := time.Now().Add(time.Second)

	ref, err := repo.Head()
	require.NoError(t, err)
	commit, err := repo.CommitObject(ref.Hash())
	require.NoError(t, err)

	// The bug: a non-nil Author with no When committed at 0001-01-01.
	require.False(t, commit.Author.When.IsZero(), "author timestamp must not be zero")
	require.WithinRange(t, commit.Author.When, before, after)
	require.WithinRange(t, commit.Committer.When, before, after)
}

func TestGitStore_ReadReadsCommittedHEADNotWorkingTree(t *testing.T) {
	dir, _ := newRepo(t)
	store, err := NewGitStore(dir, object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)

	// Empty repo (no commits) reads as an empty set, not an error.
	got, err := store.Read(".env.versions.test")
	require.NoError(t, err)
	require.Empty(t, got)

	require.NoError(t, store.WriteAndCommit(".env.versions.test", pin.Set{"A": "reg/a:v1"}, "commit v1"))

	// An uncommitted working-tree edit must NOT be seen by Read (committed contract).
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.versions.test"), []byte("A=reg/a:v2\n"), 0o644))
	got, err = store.Read(".env.versions.test")
	require.NoError(t, err)
	require.Equal(t, pin.Set{"A": "reg/a:v1"}, got)
}
