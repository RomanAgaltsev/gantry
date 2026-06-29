package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func rollbackCfg() *config.Config {
	return &config.Config{Environments: []config.Environment{
		{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
	}}
}

// Rollback targets the most recent GREEN ledger entry older than the current pin commit,
// not merely the parent commit — so it can never redeploy a known-bad set.
func TestRollback_RestoresLastGreen(t *testing.T) {
	store := &fakeStore{
		cur:     pin.Set{"SVC_IMAGE": "reg/svc:bad"}, // current prod content
		headSHA: "cur",
		atSHA:   map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:good"}},
	}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{
		{Environment: "prod", PinCommit: "g1", Result: "ok"},      // older green
		{Environment: "prod", PinCommit: "cur", Result: "failed"}, // current, failed
	}}

	res, err := Rollback(context.Background(), rollbackCfg(), "prod", ex, nil, store, led, RollbackOptions{})
	require.NoError(t, err)
	require.Equal(t, "g1", res.ToSHA)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:good"}, store.committed)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:good"}, ex.pins)
	last := led.entries[len(led.entries)-1]
	require.Equal(t, "rollback", last.By)
	require.Equal(t, "prod", last.Environment)
}

func TestRollback_NoHistory(t *testing.T) {
	store := &fakeStore{} // LatestCommit → ErrNoHistory
	_, err := Rollback(context.Background(), rollbackCfg(), "prod", &fakeExec{}, nil, store, &fakeLedger{}, RollbackOptions{})
	require.ErrorContains(t, err, "no pin history")
}

// When the only green entry is the current deploy, there is nothing earlier to roll back to.
func TestRollback_NoEarlierGreen(t *testing.T) {
	store := &fakeStore{headSHA: "cur", atSHA: map[string]pin.Set{"cur": {"A": "x"}}}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "prod", PinCommit: "cur", Result: "ok"}}}
	_, err := Rollback(context.Background(), rollbackCfg(), "prod", &fakeExec{}, nil, store, led, RollbackOptions{})
	require.ErrorContains(t, err, "no earlier green")
}

func TestRollback_EmptyTargetRefused(t *testing.T) {
	store := &fakeStore{headSHA: "cur"} // no atSHA["g1"] → ReadAt returns empty
	led := &fakeLedger{entries: []ledger.Entry{
		{Environment: "prod", PinCommit: "g1", Result: "ok"},
		{Environment: "prod", PinCommit: "cur", Result: "failed"},
	}}
	_, err := Rollback(context.Background(), rollbackCfg(), "prod", &fakeExec{}, nil, store, led, RollbackOptions{})
	require.ErrorContains(t, err, "empty")
}

func TestRollback_DryRun(t *testing.T) {
	store := &fakeStore{
		cur:     pin.Set{"SVC_IMAGE": "reg/svc:bad"},
		headSHA: "cur",
		atSHA:   map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:good"}},
	}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{
		{Environment: "prod", PinCommit: "g1", Result: "ok"},
		{Environment: "prod", PinCommit: "cur", Result: "failed"},
	}}
	res, err := Rollback(context.Background(), rollbackCfg(), "prod", ex, nil, store, led, RollbackOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, res.DryRun)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
}

// A repeat rollback when prod already holds the last good set must redeploy it rather than
// make an empty commit (go-git rejects empty commits) or oscillate forward to the bad set.
func TestRollback_NoChangeRedeploysWithoutEmptyCommit(t *testing.T) {
	store := &fakeStore{
		cur:     pin.Set{"SVC_IMAGE": "reg/svc:good"}, // already at the good set
		headSHA: "cur",
		atSHA:   map[string]pin.Set{"g1": {"SVC_IMAGE": "reg/svc:good"}},
	}
	ex := &fakeExec{}
	led := &fakeLedger{entries: []ledger.Entry{
		{Environment: "prod", PinCommit: "g1", Result: "ok"},
		{Environment: "prod", PinCommit: "cur", Result: "ok"},
	}}
	res, err := Rollback(context.Background(), rollbackCfg(), "prod", ex, nil, store, led, RollbackOptions{})
	require.NoError(t, err)
	require.Nil(t, store.committed)        // no empty commit
	require.True(t, ex.called)             // but still redeployed
	require.Equal(t, "cur", res.Committed) // keyed by the existing latest commit
}
