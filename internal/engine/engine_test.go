package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
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
	headSHA   string             // LatestCommit returns this
	atSHA     map[string]pin.Set // ReadAt lookups
	parent    map[string]string  // ParentOf lookups
}

func (s *fakeStore) Read(string) (pin.Set, error) { return s.cur, nil }
func (s *fakeStore) ReadAt(sha, _ string) (pin.Set, error) {
	if p, ok := s.atSHA[sha]; ok {
		return p, nil
	}
	return pin.Set{}, nil
}

func (s *fakeStore) WriteAndCommit(_ string, set pin.Set, msg string) (string, error) {
	s.committed, s.msg = set, msg
	s.headSHA = "newsha"
	return "newsha", nil
}

func (s *fakeStore) LatestCommit(string) (string, error) {
	if s.headSHA == "" {
		return "", ErrNoHistory
	}
	return s.headSHA, nil
}

func (s *fakeStore) ParentOf(sha string) (string, error) {
	if p, ok := s.parent[sha]; ok {
		return p, nil
	}
	return "", ErrNoParent
}

type fakeLedger struct {
	entries []ledger.Entry
}

func (l *fakeLedger) Record(e ledger.Entry) error { l.entries = append(l.entries, e); return nil }
func (l *fakeLedger) Lookup(env, sha string) (ledger.Entry, bool, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].PinCommit == sha {
			return l.entries[i], true, nil
		}
	}
	return ledger.Entry{}, false, nil
}

func (l *fakeLedger) LatestGreen(env string) (ledger.Entry, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].Result == "ok" {
			return l.entries[i], nil
		}
	}
	return ledger.Entry{}, ledger.ErrNoGreen
}

func (l *fakeLedger) History(env string) ([]ledger.Entry, error) {
	var out []ledger.Entry
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env {
			out = append(out, l.entries[i])
		}
	}
	return out, nil
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
	return executor.Result{}, stringError("ssh down")
}

type stringError string

func (e stringError) Error() string { return string(e) }

func TestSync_DeployFailureRecordsFailedSoNextSyncHeals(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	led := &fakeLedger{}

	_, err := Sync(context.Background(), cfg(), "test", f, &failExec{}, store, led, SyncOptions{})
	require.Error(t, err)
	// Pins were committed before the deploy attempt; the failure is recorded as "failed"
	// keyed by the pin commit so the next Sync self-heals (decision A2-D7).
	require.NotNil(t, store.committed)
	require.Len(t, led.entries, 1)
	require.Equal(t, "failed", led.entries[0].Result)
	require.Equal(t, "newsha", led.entries[0].PinCommit)
	require.Equal(t, "sync", led.entries[0].By)
}

func TestSync_NilExecutorErrorsNotPanics(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}

	_, err := Sync(context.Background(), cfg(), "test", f, nil, store, &fakeLedger{}, SyncOptions{})
	require.ErrorContains(t, err, "no executor")
}

func TestDeploy_NilExecutorErrorsNotPanics(t *testing.T) {
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}, headSHA: "h1"}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}

	_, err := Deploy(context.Background(), c, "test", nil, store, &fakeLedger{})
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

func TestSync_NoDiffIsNoOp(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, &fakeLedger{}, SyncOptions{})
	require.NoError(t, err)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
	require.Empty(t, res.Changes)
}

func TestSync_DryRunDoesNotCommitOrDeploy(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, &fakeLedger{}, SyncOptions{DryRun: true})
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

	_, err := Sync(context.Background(), c, "test", f, ex, store, &fakeLedger{}, SyncOptions{})
	require.NoError(t, err)
	// forge component advanced to v2; explicit pg carried forward, never polled.
	require.Equal(t, "reg/svc:v2", ex.pins["SVC_IMAGE"])
	require.Equal(t, "postgres:16.4", ex.pins["POSTGRES_IMAGE"])
	require.Equal(t, "postgres:16.4", store.committed["POSTGRES_IMAGE"])
}

func TestDeploy_ReconcilesCurrentPinFile(t *testing.T) {
	store := &fakeStore{
		cur: pin.Set{
			"SVC_IMAGE":      "reg/svc:v1",
			"POSTGRES_IMAGE": "postgres:16.4",
		},
		headSHA: "h1",
	}
	ex := &fakeExec{}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}

	res, err := Deploy(context.Background(), c, "test", ex, store, &fakeLedger{})
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

	_, err := Deploy(context.Background(), c, "test", ex, store, &fakeLedger{})
	require.Error(t, err)
	require.False(t, ex.called)
}

func TestSync_DiffDeploysCommitsAndRecords(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}
	led := &fakeLedger{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, led, SyncOptions{})
	require.NoError(t, err)
	require.True(t, ex.called)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, store.committed)
	require.Len(t, res.Changes, 1)
	require.Len(t, led.entries, 1)
	require.Equal(t, "ok", led.entries[0].Result)
	require.Equal(t, "newsha", led.entries[0].PinCommit)
	require.Equal(t, "sync", led.entries[0].By)
	require.Equal(t, "unknown", led.entries[0].Healthy)
}

func TestSync_NoDiff_Green_IsNoOp(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}, headSHA: "h1"}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "h1", Result: "ok"}}}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, led, SyncOptions{})
	require.NoError(t, err)
	require.False(t, ex.called)
	require.False(t, res.Recovered)
	require.Empty(t, res.Changes)
}

func TestSync_NoDiff_NotGreen_SelfHeals(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}, headSHA: "h1"}
	ex := &fakeExec{}
	// h1 has a failed entry only → must redeploy and record a fresh outcome
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "h1", Result: "failed"}}}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, store, led, SyncOptions{})
	require.NoError(t, err)
	require.True(t, ex.called)
	require.True(t, res.Recovered)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, ex.pins)
	require.Equal(t, "ok", led.entries[len(led.entries)-1].Result)
	require.Equal(t, "h1", led.entries[len(led.entries)-1].PinCommit)
}
