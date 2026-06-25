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

func newPinRepo(t *testing.T) (string, PinStore) {
	t.Helper()
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	store, err := NewGitStore(dir, object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)
	return dir, store
}

func TestGitStore_ReadAt_LatestCommit_ParentOf(t *testing.T) {
	_, store := newPinRepo(t)
	pinFile := ".env.versions.test"

	sha1, err := store.WriteAndCommit(pinFile, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, "pin v1")
	require.NoError(t, err)
	sha2, err := store.WriteAndCommit(pinFile, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, "pin v2")
	require.NoError(t, err)
	require.NotEqual(t, sha1, sha2)

	latest, err := store.LatestCommit(pinFile)
	require.NoError(t, err)
	require.Equal(t, sha2, latest)

	at1, err := store.ReadAt(sha1, pinFile)
	require.NoError(t, err)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, at1)

	parent, err := store.ParentOf(sha2)
	require.NoError(t, err)
	require.Equal(t, sha1, parent)
}

func TestGitStore_LatestCommit_NoHistory(t *testing.T) {
	_, store := newPinRepo(t)
	_, err := store.LatestCommit(".env.versions.test")
	require.ErrorIs(t, err, ErrNoHistory)
}

func TestGitStore_ParentOf_FirstCommit(t *testing.T) {
	_, store := newPinRepo(t)
	sha1, err := store.WriteAndCommit(".env.versions.test", pin.Set{"A": "x"}, "first")
	require.NoError(t, err)
	_, err = store.ParentOf(sha1)
	require.ErrorIs(t, err, ErrNoParent)
}

// path-filtered history must skip unrelated commits between pin changes
func TestGitStore_LatestCommit_SkipsUnrelated(t *testing.T) {
	dir, store := newPinRepo(t)
	pinFile := ".env.versions.test"
	sha1, err := store.WriteAndCommit(pinFile, pin.Set{"A": "v1"}, "pin v1")
	require.NoError(t, err)

	// an unrelated commit that does not touch the pin file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o644))
	repo, _ := git.PlainOpen(dir)
	wt, _ := repo.Worktree()
	_, _ = wt.Add("other.txt")
	_, err = wt.Commit("unrelated", &git.CommitOptions{Author: &object.Signature{Name: "x", Email: "x@y"}})
	require.NoError(t, err)

	latest, err := store.LatestCommit(pinFile)
	require.NoError(t, err)
	require.Equal(t, sha1, latest) // last commit that touched the pin file
}
