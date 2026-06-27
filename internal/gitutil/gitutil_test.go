package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

func repoWT(t *testing.T) (string, *git.Worktree) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	return dir, wt
}

func commitFile(t *testing.T, dir string, wt *git.Worktree, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	_, err := wt.Add(name)
	require.NoError(t, err)
	_, err = wt.Commit("seed", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@e"}})
	require.NoError(t, err)
}

func TestAssertOwnsIndex_CleanPasses(t *testing.T) {
	dir, wt := repoWT(t)
	commitFile(t, dir, wt, "seed.txt", "x")
	require.NoError(t, AssertOwnsIndex(wt, "pin.env"))
}

func TestAssertOwnsIndex_StagedOtherFails(t *testing.T) {
	dir, wt := repoWT(t)
	commitFile(t, dir, wt, "seed.txt", "x")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("y"), 0o600))
	_, err := wt.Add("other.txt")
	require.NoError(t, err)
	require.ErrorContains(t, AssertOwnsIndex(wt, "pin.env"), "other.txt")
}

func TestAssertOwnsIndex_StagedAllowPathPasses(t *testing.T) {
	dir, wt := repoWT(t)
	commitFile(t, dir, wt, "seed.txt", "x")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pin.env"), []byte("A=1"), 0o600))
	_, err := wt.Add("pin.env")
	require.NoError(t, err)
	require.NoError(t, AssertOwnsIndex(wt, "pin.env"))
}

func TestAssertOwnsIndex_UntrackedOtherPasses(t *testing.T) {
	dir, wt := repoWT(t)
	commitFile(t, dir, wt, "seed.txt", "x")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("z"), 0o600))
	require.NoError(t, AssertOwnsIndex(wt, "pin.env"))
}

func TestAssertOwnsIndex_NoCommitsPasses(t *testing.T) {
	_, wt := repoWT(t)
	require.NoError(t, AssertOwnsIndex(wt, "pin.env"))
}
