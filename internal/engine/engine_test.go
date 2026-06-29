package engine

import (
	"context"
	"errors"
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
	resolve   map[string]string  // Resolve lookups (unmapped revs return unchanged)
}

func (s *fakeStore) Read(string) (pin.Set, error) { return s.cur, nil }

func (s *fakeStore) Resolve(rev string) (string, error) {
	if full, ok := s.resolve[rev]; ok {
		return full, nil
	}
	return rev, nil
}

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
	entries   []ledger.Entry
	recordErr error // when set, Record fails with it
}

func (l *fakeLedger) Record(e ledger.Entry) error {
	if l.recordErr != nil {
		return l.recordErr
	}
	l.entries = append(l.entries, e)
	return nil
}

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

func (l *fakeLedger) LatestHealthy(env string) (ledger.Entry, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].Result == "ok" && l.entries[i].Healthy == "true" {
			return l.entries[i], nil
		}
	}
	return ledger.Entry{}, ledger.ErrNoGreen
}

type fakeExec struct {
	called bool
	pins   pin.Set
	commit string
}

func (e *fakeExec) Deploy(_ context.Context, p executor.Plan) (executor.Result, error) {
	e.called, e.pins, e.commit = true, p.Pins, p.Commit
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

	_, err := Sync(context.Background(), cfg(), "test", f, &failExec{}, nil, store, led, SyncOptions{})
	require.Error(t, err)
	// Pins were committed before the deploy attempt; the failure is recorded as "failed"
	// keyed by the pin commit so the next Sync self-heals.
	require.NotNil(t, store.committed)
	require.Len(t, led.entries, 1)
	require.Equal(t, "failed", led.entries[0].Result)
	require.Equal(t, "newsha", led.entries[0].PinCommit)
	require.Equal(t, "sync", led.entries[0].By)
}

func TestSync_DeployAndRecordBothFail_SurfacesBoth(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	led := &fakeLedger{recordErr: stringError("ledger write failed")}

	_, err := Sync(context.Background(), cfg(), "test", f, &failExec{}, nil, store, led, SyncOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "ssh down")     // the deploy failure
	require.ErrorContains(t, err, "ledger write") // the record failure is not dropped
}

func TestSync_NilExecutorErrorsNotPanics(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}

	_, err := Sync(context.Background(), cfg(), "test", f, nil, nil, store, &fakeLedger{}, SyncOptions{})
	require.ErrorContains(t, err, "no executor")
}

func TestDeploy_NilExecutorErrorsNotPanics(t *testing.T) {
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}, headSHA: "h1"}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}

	_, err := Deploy(context.Background(), c, "test", nil, nil, store, &fakeLedger{})
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

	res, err := Sync(context.Background(), cfg(), "test", f, ex, nil, store, &fakeLedger{}, SyncOptions{})
	require.NoError(t, err)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
	require.Empty(t, res.Changes)
}

func TestSync_DryRunDoesNotCommitOrDeploy(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, nil, store, &fakeLedger{}, SyncOptions{DryRun: true})
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

	_, err := Sync(context.Background(), c, "test", f, ex, nil, store, &fakeLedger{}, SyncOptions{})
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

	res, err := Deploy(context.Background(), c, "test", ex, nil, store, &fakeLedger{})
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

	_, err := Deploy(context.Background(), c, "test", ex, nil, store, &fakeLedger{})
	require.Error(t, err)
	require.False(t, ex.called)
}

func TestSync_DiffDeploysCommitsAndRecords(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}
	led := &fakeLedger{}

	res, err := Sync(context.Background(), cfg(), "test", f, ex, nil, store, led, SyncOptions{})
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

	res, err := Sync(context.Background(), cfg(), "test", f, ex, nil, store, led, SyncOptions{})
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

	res, err := Sync(context.Background(), cfg(), "test", f, ex, nil, store, led, SyncOptions{})
	require.NoError(t, err)
	require.True(t, ex.called)
	require.True(t, res.Recovered)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, ex.pins)
	require.Equal(t, "ok", led.entries[len(led.entries)-1].Result)
	require.Equal(t, "h1", led.entries[len(led.entries)-1].PinCommit)
}

type fakeVerifier struct{ err error }

func (v fakeVerifier) Verify(context.Context) error { return v.err }

func TestDeployAndRecord_VerifyMatrix(t *testing.T) {
	base := func() (*fakeStore, *fakeLedger) {
		return &fakeStore{cur: pin.Set{"K": "img:v1"}, headSHA: "sha1"}, &fakeLedger{}
	}
	pins := pin.Set{"K": "img:v1"}

	t.Run("deploy ok, verify pass -> ok/true", func(t *testing.T) {
		_, led := base()
		err := deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &fakeExec{}, fakeVerifier{nil}, led)
		require.NoError(t, err)
		require.Equal(t, "ok", led.entries[0].Result)
		require.Equal(t, "true", led.entries[0].Healthy)
	})

	t.Run("deploy ok, verify fail -> failed/false + error", func(t *testing.T) {
		_, led := base()
		err := deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &fakeExec{}, fakeVerifier{errors.New("503")}, led)
		require.Error(t, err)
		require.Contains(t, err.Error(), "verify")
		require.Equal(t, "failed", led.entries[0].Result)
		require.Equal(t, "false", led.entries[0].Healthy)
	})

	t.Run("no verifier -> ok/unknown (A2 behavior preserved)", func(t *testing.T) {
		_, led := base()
		err := deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &fakeExec{}, nil, led)
		require.NoError(t, err)
		require.Equal(t, "ok", led.entries[0].Result)
		require.Equal(t, "unknown", led.entries[0].Healthy)
	})

	t.Run("deploy fail -> verify not run, failed/unknown", func(t *testing.T) {
		_, led := base()
		err := deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &failExec{}, fakeVerifier{errors.New("unused")}, led)
		require.Error(t, err)
		require.Contains(t, err.Error(), "deploy")
		require.Equal(t, "failed", led.entries[0].Result)
		require.Equal(t, "unknown", led.entries[0].Healthy)
	})
}

func TestDeploy_SetsPlanCommit(t *testing.T) {
	cfg := &config.Config{Environments: []config.Environment{
		{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
	}}
	store := &fakeStore{cur: pin.Set{"K": "img:v1"}, headSHA: "deadbeef"}
	fe := &fakeExec{}
	_, err := Deploy(context.Background(), cfg, "test", fe, nil, store, &fakeLedger{})
	require.NoError(t, err)
	require.Equal(t, "deadbeef", fe.commit) // Plan.Commit == the pin commit SHA
}
