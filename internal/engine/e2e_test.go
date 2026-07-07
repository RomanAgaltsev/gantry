package engine

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
)

// TestE2E_SyncPromoteRollback drives the full sync → promote → rollback composition against a
// real git-backed store and ledger with the package's existing fakes. It pins the composition
// (and the file-model rollback path) against regressions, covering review §6 Gap 3.
func TestE2E_SyncPromoteRollback(t *testing.T) {
	sig := object.Signature{Name: "gantry", Email: "gantry@local"}
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	store, err := NewGitStore(dir, sig)
	require.NoError(t, err)
	led, err := ledger.NewGitLedger(dir, sig)
	require.NoError(t, err)

	c := &config.Config{
		Components: []config.Component{{
			ID:      "svc",
			Project: "g/svc",
			PinKey:  "SVC_IMAGE",
			Source:  config.ComponentSource{Forge: "release"},
		}},
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
			{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
		},
	}
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	ex := &fakeExec{}

	// 1) sync test @v1 → green; promote v1 to prod (first prod green).
	sr, err := Sync(context.Background(), c, "test", f, ex, nil, store, led, SyncOptions{})
	require.NoError(t, err)
	require.True(t, sr.Deployed)
	_, err = Promote(context.Background(), c, "test", "prod", "", ex, nil, store, led, PromoteOptions{})
	require.NoError(t, err)

	// 2) bump to v2, sync test, promote v2 to prod (second prod green).
	f.rel.ImageTag = "v2"
	_, err = Sync(context.Background(), c, "test", f, ex, nil, store, led, SyncOptions{})
	require.NoError(t, err)
	pr, err := Promote(context.Background(), c, "test", "prod", "", ex, nil, store, led, PromoteOptions{})
	require.NoError(t, err)
	require.True(t, pr.Deployed)

	// 3) rollback prod → the earlier green (v1).
	rr, err := Rollback(context.Background(), c, "prod", ex, nil, store, led, RollbackOptions{})
	require.NoError(t, err)
	require.True(t, rr.Deployed)
	require.NotEmpty(t, rr.ToSHA)
	require.Equal(t, "reg/svc:v1", ex.pins["SVC_IMAGE"]) // rolled back to the v1 pin set
}
