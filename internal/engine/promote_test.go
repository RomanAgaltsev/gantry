package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func promoteCfg() *config.Config {
	return &config.Config{
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
			{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
		},
	}
}

func TestPromote_DefaultGreenSHA(t *testing.T) {
	store := &fakeStore{atSHA: map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:v2"}}}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "g1", Result: "ok"}}}

	res, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "", ex, nil, PromoteOptions{})
	require.NoError(t, err)
	require.Equal(t, "g1", res.FromSHA)
	require.True(t, res.Deployed)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, store.committed) // snapshot written to prod
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v2"}, ex.pins)
	last := led.entries[len(led.entries)-1]
	require.Equal(t, "prod", last.Environment)
	require.Equal(t, "promote", last.By)
	require.Equal(t, "newsha", last.PinCommit)
}

func TestPromote_ResolvesShortSHA(t *testing.T) {
	store := &fakeStore{
		atSHA:   map[string]pin.Set{"fullsha": {"SVC_IMAGE": "reg/svc:v2"}},
		resolve: map[string]string{"short": "fullsha"},
	}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "fullsha", Result: "ok"}}}

	res, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "short", &fakeExec{}, nil, PromoteOptions{})
	require.NoError(t, err)
	require.Equal(t, "fullsha", res.FromSHA) // gate + snapshot used the resolved full SHA
}

func TestPromote_RefusesMissingGate(t *testing.T) {
	store := &fakeStore{atSHA: map[string]pin.Set{"x": {"SVC_IMAGE": "reg/svc:v2"}}}
	led := &fakeLedger{} // no entry for (test, x)
	_, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "x", &fakeExec{}, nil, PromoteOptions{})
	require.ErrorContains(t, err, "no deploy record")
}

func TestPromote_RefusesFailedGate(t *testing.T) {
	store := &fakeStore{atSHA: map[string]pin.Set{"x": {"SVC_IMAGE": "reg/svc:v2"}}}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "x", Result: "failed"}}}
	_, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "x", &fakeExec{}, nil, PromoteOptions{})
	require.ErrorContains(t, err, "not ok")
}

func TestPromote_NoGreenToPromote(t *testing.T) {
	store := &fakeStore{}
	led := &fakeLedger{}
	_, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "", &fakeExec{}, nil, PromoteOptions{})
	require.ErrorContains(t, err, "no green deploy")
}

func TestPromote_RequireHealthy(t *testing.T) {
	cfg := func() *config.Config {
		c := promoteCfg()
		c.Promote.RequireHealthy = true
		return c
	}

	t.Run("default path refuses ok-but-unknown source", func(t *testing.T) {
		store := &fakeStore{atSHA: map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:v2"}}}
		led := &fakeLedger{entries: []ledger.Entry{
			{Environment: "test", PinCommit: "g1", Result: "ok", Healthy: "unknown"},
		}}
		_, err := (&Engine{Cfg: cfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "", &fakeExec{}, nil, PromoteOptions{})
		require.ErrorContains(t, err, "healthy")
	})

	t.Run("default path promotes green+healthy source", func(t *testing.T) {
		store := &fakeStore{atSHA: map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:v2"}}}
		led := &fakeLedger{entries: []ledger.Entry{
			{Environment: "test", PinCommit: "g0", Result: "ok", Healthy: "unknown"},
			{Environment: "test", PinCommit: "g1", Result: "ok", Healthy: "true"},
		}}
		res, err := (&Engine{Cfg: cfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "", &fakeExec{}, nil, PromoteOptions{})
		require.NoError(t, err)
		require.Equal(t, "g1", res.FromSHA) // snapshotted the healthy entry, not the unknown one
		require.True(t, res.Deployed)
	})

	t.Run("explicit --sha refuses ok-but-unknown entry", func(t *testing.T) {
		store := &fakeStore{atSHA: map[string]pin.Set{"x": {"SVC_IMAGE": "reg/svc:v2"}}}
		led := &fakeLedger{entries: []ledger.Entry{
			{Environment: "test", PinCommit: "x", Result: "ok", Healthy: "unknown"},
		}}
		_, err := (&Engine{Cfg: cfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "x", &fakeExec{}, nil, PromoteOptions{})
		require.ErrorContains(t, err, "healthy")
	})

	t.Run("default false promotes ok-but-unknown entry (A2 behavior)", func(t *testing.T) {
		store := &fakeStore{atSHA: map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:v2"}}}
		led := &fakeLedger{entries: []ledger.Entry{
			{Environment: "test", PinCommit: "g1", Result: "ok", Healthy: "unknown"},
		}}
		res, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "", &fakeExec{}, nil, PromoteOptions{})
		require.NoError(t, err)
		require.Equal(t, "g1", res.FromSHA)
		require.True(t, res.Deployed)
	})
}

func TestPromote_DryRun(t *testing.T) {
	store := &fakeStore{atSHA: map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:v2"}}}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "g1", Result: "ok"}}}
	res, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(context.Background(), "test", "prod", "", ex, nil, PromoteOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, res.DryRun)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
	require.Len(t, led.entries, 1) // the seeded gate entry only; DryRun must not record
}

// TestPromote_OnlySubset promotes just the listed pin keys, carrying the target's other
// pins forward unchanged. The subset was never validated together as a unit (review §9.5).
func TestPromote_OnlySubset(t *testing.T) {
	store := &fakeStore{
		cur:   pin.Set{"A_IMAGE": "reg/a:v1", "B_IMAGE": "reg/b:v1"}, // prod current
		atSHA: map[string]pin.Set{"g1": {"A_IMAGE": "reg/a:v2", "B_IMAGE": "reg/b:v2"}},
	}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "g1", Result: "ok"}}}

	res, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(
		context.Background(), "test", "prod", "", ex, nil, PromoteOptions{Only: []string{"A_IMAGE"}})
	require.NoError(t, err)
	require.True(t, res.Deployed)
	// Only A advanced to v2; B carried forward from prod's current v1.
	require.Equal(t, pin.Set{"A_IMAGE": "reg/a:v2", "B_IMAGE": "reg/b:v1"}, store.committed)
}

// TestPromote_OnlyMissingKeyErrors refuses an --only key the source does not pin, rather
// than silently promoting nothing for it.
func TestPromote_OnlyMissingKeyErrors(t *testing.T) {
	store := &fakeStore{
		cur:   pin.Set{"A_IMAGE": "reg/a:v1"},
		atSHA: map[string]pin.Set{"g1": {"A_IMAGE": "reg/a:v2"}},
	}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "test", PinCommit: "g1", Result: "ok"}}}

	_, err := (&Engine{Cfg: promoteCfg(), Store: store, Ledger: led}).Promote(
		context.Background(), "test", "prod", "", &fakeExec{}, nil, PromoteOptions{Only: []string{"NOPE"}})
	require.ErrorContains(t, err, "not present in the promoted pin set")
}
