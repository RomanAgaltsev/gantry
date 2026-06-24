package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type fakeForge struct{ rel forge.Release }

func (f fakeForge) LatestRelease(_ context.Context, c forge.Component) (forge.Release, error) {
	r := f.rel
	r.Component = c.ID
	return r, nil
}

type fakeStore struct {
	cur       pin.Set
	committed pin.Set
	msg       string
}

func (s *fakeStore) Read(string) (pin.Set, error) { return s.cur, nil }
func (s *fakeStore) WriteAndCommit(_ string, set pin.Set, msg string) error {
	s.committed, s.msg = set, msg
	return nil
}

type fakeExec struct {
	called bool
	pins   pin.Set
}

func (e *fakeExec) Deploy(_ context.Context, p executor.Plan) (executor.Result, error) {
	e.called, e.pins = true, p.Pins
	return executor.Result{Changed: true}, nil
}

type failExec struct{}

func (e *failExec) Deploy(context.Context, executor.Plan) (executor.Result, error) {
	return executor.Result{}, errString("ssh down")
}

type errString string

func (e errString) Error() string { return string(e) }

func TestSync_DeployFailureSurfacesRecoveryPath(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}

	_, err := Sync(context.Background(), cfg(), "test", f, &failExec{}, store, SyncOptions{})
	require.Error(t, err)
	// Pins were committed; the message must tell the operator how to recover.
	require.NotNil(t, store.committed)
	require.ErrorContains(t, err, "gantry deploy")
}

func TestSync_NilExecutorErrorsNotPanics(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}

	_, err := Sync(context.Background(), cfg(), "test", f, nil, store, SyncOptions{})
	require.ErrorContains(t, err, "no executor")
	require.Nil(t, store.committed) // bailed before committing
}

func TestDeploy_NilExecutorErrorsNotPanics(t *testing.T) {
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}

	_, err := Deploy(context.Background(), c, "test", nil, store)
	require.ErrorContains(t, err, "no executor")
}

func cfg() *config.Config {
	return &config.Config{
		Components: []config.Component{{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE"}},
		Environments: []config.Environment{{
			Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
		}},
	}
}

func TestSync_DiffDeploysAndCommits(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, SyncOptions{})
	require.NoError(t, err)
	require.True(t, ex.called)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, ex.pins)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, store.committed)
	require.Len(t, res.Changes, 1)
}

func TestSync_NoDiffIsNoOp(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, SyncOptions{})
	require.NoError(t, err)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
	require.Empty(t, res.Changes)
}

func TestSync_DryRunDoesNotCommitOrDeploy(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, SyncOptions{DryRun: true})
	require.NoError(t, err)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
	require.Len(t, res.Changes, 1)
}

func TestSync_SkipsExplicitComponents(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{
		"SVC_IMAGE":      "reg/svc:v1",
		"POSTGRES_IMAGE": "postgres:16.4",
	}}
	ex := &fakeExec{}
	c := &config.Config{
		Components: []config.Component{
			{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
			{ID: "pg", PinKey: "POSTGRES_IMAGE", Source: config.ComponentSource{Pin: "explicit"}},
		},
		Environments: []config.Environment{{
			Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
		}},
	}

	_, err := Sync(context.Background(), c, "test", f, ex, store, SyncOptions{})
	require.NoError(t, err)
	// forge component advanced to v2; explicit pg carried forward, never polled.
	require.Equal(t, "reg/svc:v2", ex.pins["SVC_IMAGE"])
	require.Equal(t, "postgres:16.4", ex.pins["POSTGRES_IMAGE"])
	require.Equal(t, "postgres:16.4", store.committed["POSTGRES_IMAGE"])
}

func TestDeploy_ReconcilesCurrentPinFile(t *testing.T) {
	store := &fakeStore{cur: pin.Set{
		"SVC_IMAGE":      "reg/svc:v1",
		"POSTGRES_IMAGE": "postgres:16.4",
	}}
	ex := &fakeExec{}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}

	res, err := Deploy(context.Background(), c, "test", ex, store)
	require.NoError(t, err)
	require.True(t, ex.called)
	require.Equal(t, store.cur, ex.pins) // whole pin file, both sources
	require.True(t, res.Deployed)
}

func TestDeploy_EmptyPinFileErrors(t *testing.T) {
	store := &fakeStore{cur: pin.Set{}}
	ex := &fakeExec{}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", PinFile: ".env.versions.test",
	}}}

	_, err := Deploy(context.Background(), c, "test", ex, store)
	require.Error(t, err)
	require.False(t, ex.called)
}
