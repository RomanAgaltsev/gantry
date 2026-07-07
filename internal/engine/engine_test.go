package engine

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/logging"
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
	byFile    map[string]pin.Set // when set, Read returns per-pinFile pins (matrix tests)
	committed pin.Set
	msg       string
	headSHA   string             // LatestCommit returns this
	atSHA     map[string]pin.Set // ReadAt lookups
	parent    map[string]string  // ParentOf lookups
	resolve   map[string]string  // Resolve lookups (unmapped revs return unchanged)
}

func (s *fakeStore) Read(_ context.Context, pinFile string) (pin.Set, error) {
	if s.byFile != nil {
		return s.byFile[pinFile], nil
	}
	return s.cur, nil
}

func (s *fakeStore) Resolve(_ context.Context, rev string) (string, error) {
	if full, ok := s.resolve[rev]; ok {
		return full, nil
	}
	return rev, nil
}

func (s *fakeStore) ReadAt(_ context.Context, sha, _ string) (pin.Set, error) {
	if p, ok := s.atSHA[sha]; ok {
		return p, nil
	}
	return pin.Set{}, nil
}

func (s *fakeStore) WriteAndCommit(_ context.Context, _ string, set pin.Set, msg string) (string, error) {
	s.committed, s.msg = set, msg
	s.headSHA = "newsha"
	return "newsha", nil
}

func (s *fakeStore) LatestCommit(context.Context, string) (string, error) {
	if s.headSHA == "" {
		return "", ErrNoHistory
	}
	return s.headSHA, nil
}

func (s *fakeStore) ParentOf(_ context.Context, sha string) (string, error) {
	if p, ok := s.parent[sha]; ok {
		return p, nil
	}
	return "", ErrNoParent
}

type fakeLedger struct {
	entries   []ledger.Entry
	recordErr error // when set, Record fails with it
}

func (l *fakeLedger) Record(_ context.Context, e ledger.Entry) error {
	if l.recordErr != nil {
		return l.recordErr
	}
	l.entries = append(l.entries, e)
	return nil
}

func (l *fakeLedger) Lookup(_ context.Context, env, sha string) (ledger.Entry, bool, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].PinCommit == sha {
			return l.entries[i], true, nil
		}
	}
	return ledger.Entry{}, false, nil
}

func (l *fakeLedger) LatestGreen(_ context.Context, env string) (ledger.Entry, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].Result == ledger.ResultOK {
			return l.entries[i], nil
		}
	}
	return ledger.Entry{}, ledger.ErrNoGreen
}

func (l *fakeLedger) History(_ context.Context, env string) ([]ledger.Entry, error) {
	var out []ledger.Entry
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env {
			out = append(out, l.entries[i])
		}
	}
	return out, nil
}

func (l *fakeLedger) LatestHealthy(_ context.Context, env string) (ledger.Entry, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].Result == ledger.ResultOK && l.entries[i].Healthy == ledger.HealthTrue {
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

	_, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", &failExec{}, nil, SyncOptions{})
	require.Error(t, err)
	// Pins were committed before the deploy attempt; the failure is recorded as "failed"
	// keyed by the pin commit so the next Sync self-heals.
	require.NotNil(t, store.committed)
	require.Len(t, led.entries, 1)
	require.Equal(t, ledger.ResultFailed, led.entries[0].Result)
	require.Equal(t, "newsha", led.entries[0].PinCommit)
	require.Equal(t, "sync", led.entries[0].By)
}

func TestSync_DeployAndRecordBothFail_SurfacesBoth(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	led := &fakeLedger{recordErr: stringError("ledger write failed")}

	_, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", &failExec{}, nil, SyncOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "ssh down")     // the deploy failure
	require.ErrorContains(t, err, "ledger write") // the record failure is not dropped
}

func TestSync_NilExecutorErrorsNotPanics(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}

	_, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: &fakeLedger{}}).Sync(context.Background(), "test", nil, nil, SyncOptions{})
	require.ErrorContains(t, err, "no executor")
}

func TestDeploy_NilExecutorErrorsNotPanics(t *testing.T) {
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}, headSHA: "h1"}
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}

	_, err := (&Engine{Cfg: c, Store: store, Ledger: &fakeLedger{}}).Deploy(context.Background(), "test", nil, nil)
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

func TestPrune_RemovesOrphanAndRedeploys(t *testing.T) {
	// cfg() declares only SVC_IMAGE; OLD_IMAGE is an orphan in the pin file.
	store := &fakeStore{
		cur:     pin.Set{"SVC_IMAGE": "reg/svc:v1", "OLD_IMAGE": "reg/x:v0"},
		headSHA: "h1",
	}
	ex := &fakeExec{}
	res, err := (&Engine{Cfg: cfg(), Store: store, Ledger: &fakeLedger{}}).Prune(context.Background(), "test", ex, nil, PruneOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"OLD_IMAGE"}, res.Removed)
	require.True(t, res.Deployed)
	require.True(t, ex.called)
	require.NotContains(t, store.committed, "OLD_IMAGE") // committed set dropped the orphan
	require.Equal(t, "reg/svc:v1", store.committed["SVC_IMAGE"])
}

func TestDeploy_WarnsOnMissingPinKey(t *testing.T) {
	var buf bytes.Buffer
	ctx := logging.Into(context.Background(), slog.New(slog.NewTextHandler(&buf, nil)))
	c := &config.Config{
		Components: []config.Component{
			{ID: "a", PinKey: "A_IMAGE", Source: config.ComponentSource{Forge: "release"}},
			{ID: "b", PinKey: "B_IMAGE", Source: config.ComponentSource{Forge: "release"}},
		},
		Environments: []config.Environment{{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"}},
	}
	store := &fakeStore{cur: pin.Set{"A_IMAGE": "reg/a:v1"}, headSHA: "h1"} // B_IMAGE missing
	_, err := (&Engine{Cfg: c, Store: store, Ledger: &fakeLedger{}}).Deploy(ctx, "test", &fakeExec{}, nil)
	require.NoError(t, err) // still deploys (warning, not refusal)
	require.Contains(t, buf.String(), "B_IMAGE")
	require.Contains(t, buf.String(), "missing")
}

func TestSync_NoDiffIsNoOp(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: &fakeLedger{}}).Sync(context.Background(), "test", ex, nil, SyncOptions{})
	require.NoError(t, err)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
	require.Empty(t, res.Changes)
}

func TestSync_DryRunDoesNotCommitOrDeploy(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}

	res, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: &fakeLedger{}}).Sync(context.Background(), "test", ex, nil, SyncOptions{DryRun: true})
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

	_, err := (&Engine{Cfg: c, Forge: f, Store: store, Ledger: &fakeLedger{}}).Sync(context.Background(), "test", ex, nil, SyncOptions{})
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

	res, err := (&Engine{Cfg: c, Store: store, Ledger: &fakeLedger{}}).Deploy(context.Background(), "test", ex, nil)
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

	_, err := (&Engine{Cfg: c, Store: store, Ledger: &fakeLedger{}}).Deploy(context.Background(), "test", ex, nil)
	require.Error(t, err)
	require.False(t, ex.called)
}

func TestSync_DiffDeploysCommitsAndRecords(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
	ex := &fakeExec{}
	led := &fakeLedger{}

	res, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", ex, nil, SyncOptions{})
	require.NoError(t, err)
	require.True(t, ex.called)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, store.committed)
	require.Len(t, res.Changes, 1)
	require.Len(t, led.entries, 1)
	require.Equal(t, ledger.ResultOK, led.entries[0].Result)
	require.Equal(t, "newsha", led.entries[0].PinCommit)
	require.Equal(t, "sync", led.entries[0].By)
	require.Equal(t, ledger.HealthUnknown, led.entries[0].Healthy)
}

func TestSync_NoDiff_Green_IsNoOp(t *testing.T) {
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}
	store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}, headSHA: "h1"}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "h1", Result: "ok"}}}

	res, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", ex, nil, SyncOptions{})
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

	res, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", ex, nil, SyncOptions{})
	require.NoError(t, err)
	require.True(t, ex.called)
	require.True(t, res.Recovered)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, ex.pins)
	require.Equal(t, ledger.ResultOK, led.entries[len(led.entries)-1].Result)
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
		vfd, err := (&Engine{Ledger: led}).deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &fakeExec{}, fakeVerifier{nil})
		require.NoError(t, err)
		require.False(t, vfd)
		require.Equal(t, ledger.ResultOK, led.entries[0].Result)
		require.Equal(t, ledger.HealthTrue, led.entries[0].Healthy)
	})

	t.Run("deploy ok, verify fail -> failed/false + error", func(t *testing.T) {
		_, led := base()
		vfd, err := (&Engine{Ledger: led}).deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &fakeExec{}, fakeVerifier{errors.New("503")})
		require.Error(t, err)
		require.True(t, vfd) // the verify step failed
		require.Contains(t, err.Error(), "verify")
		require.Equal(t, ledger.ResultFailed, led.entries[0].Result)
		require.Equal(t, ledger.HealthFalse, led.entries[0].Healthy)
	})

	t.Run("no verifier -> ok/unknown (A2 behavior preserved)", func(t *testing.T) {
		_, led := base()
		vfd, err := (&Engine{Ledger: led}).deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &fakeExec{}, nil)
		require.NoError(t, err)
		require.False(t, vfd)
		require.Equal(t, ledger.HealthUnknown, led.entries[0].Healthy)
	})

	t.Run("deploy fail -> verify not run, failed/unknown", func(t *testing.T) {
		_, led := base()
		vfd, err := (&Engine{Ledger: led}).deployAndRecord(context.Background(), "test", ".env", pins, "sha1", "deploy", &failExec{}, fakeVerifier{errors.New("unused")})
		require.Error(t, err)
		require.False(t, vfd) // a deploy failure is not a verify failure
		require.Contains(t, err.Error(), "deploy")
		require.Equal(t, ledger.ResultFailed, led.entries[0].Result)
		require.Equal(t, ledger.HealthUnknown, led.entries[0].Healthy)
	})
}

// TestSync_ThreadsVerifier guards the seam that once silently dropped the verifier: a
// configured probe must actually run through Sync (not just through deployAndRecord).
func TestSync_ThreadsVerifier(t *testing.T) {
	t.Run("passing probe records healthy=true", func(t *testing.T) {
		f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
		store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
		led := &fakeLedger{}

		_, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", &fakeExec{}, fakeVerifier{nil}, SyncOptions{})
		require.NoError(t, err)
		require.Equal(t, ledger.HealthTrue, led.entries[0].Healthy)
	})

	t.Run("failing probe fails the sync and records healthy=false", func(t *testing.T) {
		f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v2"}}
		store := &fakeStore{cur: pin.Set{"SVC_IMAGE": "reg/svc:v1"}}
		led := &fakeLedger{}

		_, err := (&Engine{Cfg: cfg(), Forge: f, Store: store, Ledger: led}).Sync(context.Background(), "test", &fakeExec{}, fakeVerifier{errors.New("503")}, SyncOptions{})
		require.ErrorContains(t, err, "verify")
		require.Equal(t, ledger.ResultFailed, led.entries[0].Result)
		require.Equal(t, ledger.HealthFalse, led.entries[0].Healthy)
	})
}

// TestDeploy_ThreadsVerifier is the same guard for the Deploy path.
func TestDeploy_ThreadsVerifier(t *testing.T) {
	store := &fakeStore{cur: pin.Set{"K": "img:v1"}, headSHA: "h1"}
	c := &config.Config{Environments: []config.Environment{
		{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
	}}
	led := &fakeLedger{}

	_, err := (&Engine{Cfg: c, Store: store, Ledger: led}).Deploy(context.Background(), "test", &fakeExec{}, fakeVerifier{errors.New("503")})
	require.ErrorContains(t, err, "verify")
	require.Equal(t, ledger.HealthFalse, led.entries[0].Healthy)
}

func TestDeploy_SetsPlanCommit(t *testing.T) {
	cfg := &config.Config{Environments: []config.Environment{
		{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
	}}
	store := &fakeStore{cur: pin.Set{"K": "img:v1"}, headSHA: "deadbeef"}
	fe := &fakeExec{}
	_, err := (&Engine{Cfg: cfg, Store: store, Ledger: &fakeLedger{}}).Deploy(context.Background(), "test", fe, nil)
	require.NoError(t, err)
	require.Equal(t, "deadbeef", fe.commit) // Plan.Commit == the pin commit SHA
}

type fakeSlotExec struct {
	live       string
	switchedTo string
}

func (e *fakeSlotExec) Deploy(context.Context, executor.Plan) (executor.Result, error) {
	return executor.Result{Changed: true}, nil
}
func (e *fakeSlotExec) Slots() (string, string)                  { return "blue", "green" }
func (e *fakeSlotExec) LiveSlot(context.Context) (string, error) { return e.live, nil }
func (e *fakeSlotExec) SwitchTo(_ context.Context, slot string) error {
	e.switchedTo = slot
	return nil
}

func bgCfg() *config.Config {
	return &config.Config{Environments: []config.Environment{
		{Name: "front", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.front"},
	}}
}

func TestSwitch_GateAndFlip(t *testing.T) {
	cfg := bgCfg()
	store := &fakeStore{headSHA: "h1"}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "front", PinCommit: "h1", Result: "ok"}}}
	se := &fakeSlotExec{live: "blue"} // live blue -> idle green
	res, err := (&Engine{Cfg: cfg, Store: store, Ledger: led}).Switch(context.Background(), "front", se, nil)
	require.NoError(t, err)
	require.Equal(t, "green", se.switchedTo)
	require.Equal(t, "blue", res.From)
	require.Equal(t, "green", res.To)
	require.Equal(t, "switch", led.entries[len(led.entries)-1].By)
}

func TestSwitch_GateRefusesUnstaged(t *testing.T) {
	cfg := bgCfg()
	store := &fakeStore{headSHA: "h1"}
	led := &fakeLedger{} // no ok entry for h1
	_, err := (&Engine{Cfg: cfg, Store: store, Ledger: led}).Switch(context.Background(), "front", &fakeSlotExec{live: "blue"}, nil)
	require.Error(t, err)
}

func TestSwitch_NonSlotExecutor(t *testing.T) {
	_, err := (&Engine{Cfg: bgCfg(), Store: &fakeStore{headSHA: "h1"}, Ledger: &fakeLedger{}}).Switch(context.Background(), "front", &fakeExec{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blue-green")
}

func TestRollback_BlueGreenFlipsBack(t *testing.T) {
	cfg := bgCfg()
	store := &fakeStore{headSHA: "h2"}
	led := &fakeLedger{}
	se := &fakeSlotExec{live: "green"} // green live -> roll back to blue
	res, err := (&Engine{Cfg: cfg, Store: store, Ledger: led}).Rollback(context.Background(), "front", se, nil, RollbackOptions{})
	require.NoError(t, err)
	require.Equal(t, "blue", se.switchedTo)
	require.Equal(t, "blue", res.Slot)
	require.True(t, res.Deployed)
	require.Equal(t, "rollback", led.entries[len(led.entries)-1].By)
	require.Equal(t, ledger.HealthUnknown, led.entries[len(led.entries)-1].Healthy) // flip does not verify health
}

// seqVerifier returns errs[i] for call i, then nil. Lets a test fail the initial deploy's
// verify but pass the auto-rollback's verify of the restored (known-good) set.
type seqVerifier struct {
	errs []error
	i    int
}

func (v *seqVerifier) Verify(context.Context) error {
	if v.i < len(v.errs) {
		e := v.errs[v.i]
		v.i++
		return e
	}
	return nil
}

func rollbackCfgEng() *config.Config {
	return &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
		Verify:          []config.VerifyProbe{{Kind: "http", URL: "x"}},
		VerifyOnFailure: "rollback",
	}}}
}

func TestDeploy_AutoRollbackOnVerifyFailure(t *testing.T) {
	store := &fakeStore{
		cur:     pin.Set{"K": "img:v2"},
		headSHA: "bad",
		atSHA:   map[string]pin.Set{"good": {"K": "img:v1"}},
	}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "good", Result: "ok"}}}
	vf := &seqVerifier{errs: []error{errors.New("503")}} // bad deploy fails; restored set passes

	res, err := (&Engine{Cfg: rollbackCfgEng(), Store: store, Ledger: led}).Deploy(context.Background(), "test", &fakeExec{}, vf)
	require.Error(t, err)
	require.ErrorContains(t, err, "rolled back")
	require.True(t, res.AutoRolledBack)
	require.Equal(t, "good", res.RolledBackTo)
	require.Equal(t, ledger.ResultFailed, led.entries[1].Result) // the bad deploy
	last := led.entries[len(led.entries)-1]
	require.Equal(t, ledger.ResultOK, last.Result)
	require.Equal(t, "auto-rollback", last.By)
}

func TestDeploy_HoldOnVerifyFailure(t *testing.T) {
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
		Verify: []config.VerifyProbe{{Kind: "http", URL: "x"}}, // default hold
	}}}
	store := &fakeStore{cur: pin.Set{"K": "img:v2"}, headSHA: "bad"}
	led := &fakeLedger{}

	res, err := (&Engine{Cfg: c, Store: store, Ledger: led}).Deploy(context.Background(), "test", &fakeExec{}, fakeVerifier{errors.New("503")})
	require.Error(t, err)
	require.False(t, res.AutoRolledBack)
	require.Len(t, led.entries, 1)
	require.Equal(t, ledger.ResultFailed, led.entries[0].Result)
}

func TestDeploy_AutoRollbackNoPriorGreen(t *testing.T) {
	store := &fakeStore{cur: pin.Set{"K": "img:v2"}, headSHA: "bad"} // no prior green in ledger
	led := &fakeLedger{}
	vf := &seqVerifier{errs: []error{errors.New("503")}}

	res, err := (&Engine{Cfg: rollbackCfgEng(), Store: store, Ledger: led}).Deploy(context.Background(), "test", &fakeExec{}, vf)
	require.Error(t, err)
	require.ErrorContains(t, err, "auto-rollback")
	require.False(t, res.AutoRolledBack)
}

func TestDeploy_DeployFailureIsNotAutoRolledBack(t *testing.T) {
	store := &fakeStore{
		cur: pin.Set{"K": "img:v2"}, headSHA: "bad",
		atSHA: map[string]pin.Set{"good": {"K": "img:v1"}},
	}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "good", Result: "ok"}}}

	res, err := (&Engine{Cfg: rollbackCfgEng(), Store: store, Ledger: led}).Deploy(context.Background(), "test", &failExec{}, fakeVerifier{nil})
	require.Error(t, err)
	require.False(t, res.AutoRolledBack) // deploy failure != verify failure
	require.Equal(t, ledger.ResultFailed, led.entries[len(led.entries)-1].Result)
	require.NotEqual(t, "auto-rollback", led.entries[len(led.entries)-1].By)
}

func TestRollback_DoesNotAutoRollback(t *testing.T) {
	// A manual rollback whose own verify fails must NOT trigger a second rollback.
	store := &fakeStore{
		cur: pin.Set{"K": "img:v2"}, headSHA: "bad",
		atSHA: map[string]pin.Set{"good": {"K": "img:v1"}},
	}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "good", Result: "ok"}}}
	vf := fakeVerifier{errors.New("503")} // always fails

	_, err := (&Engine{Cfg: rollbackCfgEng(), Store: store, Ledger: led}).Rollback(context.Background(), "test", &fakeExec{}, vf, RollbackOptions{})
	require.Error(t, err)
	for _, e := range led.entries {
		require.NotEqual(t, "auto-rollback", e.By)
	}
}

func TestDeploy_HoldSetsVerifyFailed(t *testing.T) {
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
		Verify: []config.VerifyProbe{{Kind: "http", URL: "x"}}, // default hold
	}}}
	store := &fakeStore{cur: pin.Set{"K": "img:v2"}, headSHA: "bad"}
	res, err := (&Engine{Cfg: c, Store: store, Ledger: &fakeLedger{}}).Deploy(context.Background(), "test", &fakeExec{}, fakeVerifier{errors.New("503")})
	require.Error(t, err)
	require.True(t, res.VerifyFailed)
	require.False(t, res.AutoRolledBack)
}

func TestDeploy_DeployFailureIsNotVerifyFailed(t *testing.T) {
	c := &config.Config{Environments: []config.Environment{{
		Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test",
	}}}
	store := &fakeStore{cur: pin.Set{"K": "img:v2"}, headSHA: "bad"}
	res, err := (&Engine{Cfg: c, Store: store, Ledger: &fakeLedger{}}).Deploy(context.Background(), "test", &failExec{}, fakeVerifier{nil})
	require.Error(t, err)
	require.False(t, res.VerifyFailed) // an SSH/deploy failure is not a verify failure
}

func TestSwitch_RefusesWhenIdleVerifyFails(t *testing.T) {
	cfg := bgCfg()
	store := &fakeStore{headSHA: "h2"}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "front", PinCommit: "h2", Result: "ok"}}}
	se := &fakeSlotExec{live: "blue"}

	_, err := (&Engine{Cfg: cfg, Store: store, Ledger: led}).Switch(context.Background(), "front", se, fakeVerifier{errors.New("503")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "verification")
	require.Equal(t, "", se.switchedTo)                               // pointer NOT flipped
	require.NotEqual(t, "switch", led.entries[len(led.entries)-1].By) // no switch record added
}

func TestSwitch_PassesWhenIdleVerifyOK(t *testing.T) {
	cfg := bgCfg()
	store := &fakeStore{headSHA: "h2"}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "front", PinCommit: "h2", Result: "ok"}}}
	se := &fakeSlotExec{live: "blue"}

	res, err := (&Engine{Cfg: cfg, Store: store, Ledger: led}).Switch(context.Background(), "front", se, fakeVerifier{nil})
	require.NoError(t, err)
	require.Equal(t, "green", res.To)
	require.Equal(t, "green", se.switchedTo)
}

func TestDeploy_BlueGreenHoldsOnVerifyFail_NoFlip(t *testing.T) {
	cfg := bgCfg()
	cfg.Environments[0].Verify = []config.VerifyProbe{{Kind: "compose-ps"}}
	cfg.Environments[0].VerifyOnFailure = "rollback" // must be a NO-OP for blue-green
	store := &fakeStore{headSHA: "h2"}
	led := &fakeLedger{}
	se := &fakeSlotExec{live: "blue"}

	res, err := (&Engine{Cfg: cfg, Store: store, Ledger: led}).Deploy(context.Background(), "front", se, fakeVerifier{errors.New("503")})
	require.Error(t, err)
	require.False(t, res.AutoRolledBack) // no auto-rollback for a slot executor
	require.Equal(t, "", se.switchedTo)  // pointer never flipped
	for _, e := range led.entries {
		require.NotEqual(t, "auto-rollback", e.By) // no auto-rollback ledger entry
	}
}
