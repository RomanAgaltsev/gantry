package engine

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// TestGitStore_PushAndFastForwardPull drives the real RemoteSyncer capability against a bare
// local remote: A commits+pushes (remote advances), a second clone B commits and pushes, then
// A fast-forwards to it; finally divergence yields ErrNonFastForward.
func TestGitStore_PushAndFastForwardPull(t *testing.T) {
	// A bare repo is the "remote".
	remoteDir := t.TempDir()
	_, err := git.PlainInit(remoteDir, true) // bare
	require.NoError(t, err)

	// A is a fresh worktree that commits first (the bare remote starts empty), adds the bare
	// remote as "origin", and pushes its initial commit so the remote has HEAD.
	aDir := t.TempDir()
	_, err = git.PlainInit(aDir, false)
	require.NoError(t, err)
	repoA, err := git.PlainOpen(aDir)
	require.NoError(t, err)
	_, err = repoA.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
	require.NoError(t, err)

	sA, err := NewGitStore(aDir, object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)
	storeA, ok := sA.(*gitStore)
	require.True(t, ok, "NewGitStore returns a *gitStore")
	storeA.SetRemoteAuth("", "", "origin", "")

	_, err = storeA.WriteAndCommit(context.Background(), ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:v1"}, "first")
	require.NoError(t, err)
	require.NoError(t, storeA.Push(context.Background()))

	// The bare remote now has the commit; a fresh clone B sees it.
	bDir := t.TempDir()
	_, err = git.PlainClone(bDir, false, &git.CloneOptions{URL: remoteDir})
	require.NoError(t, err)

	// B commits a new pin and pushes — the remote advances.
	sB, err := NewGitStore(bDir, object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)
	storeB, ok := sB.(*gitStore)
	require.True(t, ok)
	storeB.SetRemoteAuth("", "", "origin", "")
	_, err = storeB.WriteAndCommit(context.Background(), ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:v2"}, "second")
	require.NoError(t, err)
	require.NoError(t, storeB.Push(context.Background()))

	// A fast-forwards to B's commit (no divergence).
	require.NoError(t, storeA.PullFF(context.Background()))
	got, err := storeA.Read(context.Background(), ".env.versions.test")
	require.NoError(t, err)
	require.Equal(t, "reg/svc:v2", got["SVC_IMAGE"], "A fast-forwarded to B's pin")

	// Divergence: A commits (without pushing), B commits+pushes; A's next pull is non-ff.
	_, err = storeA.WriteAndCommit(context.Background(), ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:divergent"}, "A diverges")
	require.NoError(t, err)
	_, err = storeB.WriteAndCommit(context.Background(), ".env.versions.test", pin.Set{"SVC_IMAGE": "reg/svc:other"}, "B diverges")
	require.NoError(t, err)
	require.NoError(t, storeB.Push(context.Background()))
	err = storeA.PullFF(context.Background())
	require.ErrorIs(t, err, ErrNonFastForward, "divergence must be a loud stop, not a merge")
}
