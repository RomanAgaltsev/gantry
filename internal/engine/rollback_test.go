package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func rollbackCfgRoll() *config.Config {
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

	res, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: led}).Rollback(context.Background(), "prod", ex, nil, RollbackOptions{})
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
	_, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: &fakeLedger{}}).Rollback(context.Background(), "prod", &fakeExec{}, nil, RollbackOptions{})
	require.ErrorContains(t, err, "no pin history")
}

// When the only green entry is the current deploy, there is nothing earlier to roll back to.
func TestRollback_NoEarlierGreen(t *testing.T) {
	store := &fakeStore{headSHA: "cur", atSHA: map[string]pin.Set{"cur": {"A": "x"}}}
	led := &fakeLedger{entries: []ledger.Entry{{Environment: "prod", PinCommit: "cur", Result: "ok"}}}
	_, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: led}).Rollback(context.Background(), "prod", &fakeExec{}, nil, RollbackOptions{})
	require.ErrorContains(t, err, "no earlier green")
}

func TestRollback_EmptyTargetRefused(t *testing.T) {
	store := &fakeStore{headSHA: "cur"} // no atSHA["g1"] → ReadAt returns empty
	led := &fakeLedger{entries: []ledger.Entry{
		{Environment: "prod", PinCommit: "g1", Result: "ok"},
		{Environment: "prod", PinCommit: "cur", Result: "failed"},
	}}
	_, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: led}).Rollback(context.Background(), "prod", &fakeExec{}, nil, RollbackOptions{})
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
	res, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: led}).Rollback(context.Background(), "prod", ex, nil, RollbackOptions{DryRun: true})
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
	res, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: led}).Rollback(context.Background(), "prod", ex, nil, RollbackOptions{})
	require.NoError(t, err)
	require.Nil(t, store.committed)        // no empty commit
	require.True(t, ex.called)             // but still redeployed
	require.Equal(t, "cur", res.Committed) // keyed by the existing latest commit
}

// fastFakeExec is an executor that supports --fast rollback (symlink-release style).
type fastFakeExec struct {
	flipped bool
	rel     string
}

func (e *fastFakeExec) Deploy(context.Context, executor.Plan) (executor.Result, error) {
	return executor.Result{Changed: true}, nil
}

func (e *fastFakeExec) FastRollback(context.Context) (string, error) {
	e.flipped = true
	return e.rel, nil
}

// TestRollback_FastDispatchesToFastRollbacker verifies --fast flips to the previous release
// via the FastRollbacker capability and records the outcome against the head commit, like a
// slot rollback (health unverified) (review §9 item 7).
func TestRollback_FastDispatchesToFastRollbacker(t *testing.T) {
	store := &fakeStore{headSHA: "cur"}
	ex := &fastFakeExec{rel: "prev-release"}
	led := &fakeLedger{}

	res, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: led}).Rollback(
		context.Background(), "prod", ex, nil, RollbackOptions{Fast: true})
	require.NoError(t, err)
	require.True(t, ex.flipped)
	require.True(t, res.Deployed)
	require.Equal(t, "prev-release", res.ToSHA)
	require.Len(t, led.entries, 1)
	require.Equal(t, "rollback", led.entries[0].By)
	require.Equal(t, ledger.HealthUnknown, led.entries[0].Healthy) // a flip does not verify health
}

// TestRollback_FastUnsupportedExecutorErrors refuses --fast for an executor without the
// FastRollbacker capability instead of silently doing a full redeploy.
func TestRollback_FastUnsupportedExecutorErrors(t *testing.T) {
	store := &fakeStore{headSHA: "cur"}
	_, err := (&Engine{Cfg: rollbackCfgRoll(), Store: store, Ledger: &fakeLedger{}}).Rollback(
		context.Background(), "prod", &fakeExec{}, nil, RollbackOptions{Fast: true})
	require.ErrorContains(t, err, "does not support --fast")
}
