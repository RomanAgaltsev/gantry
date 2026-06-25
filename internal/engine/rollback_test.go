package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func rollbackCfg() *config.Config {
	return &config.Config{Environments: []config.Environment{
		{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
	}}
}

func TestRollback_RestoresParentPins(t *testing.T) {
	store := &fakeStore{
		headSHA: "cur",
		parent:  map[string]string{"cur": "prev"},
		atSHA:   map[string]pin.Set{"prev": {"SVC_IMAGE": "reg/svc:v1"}},
	}
	ex := &fakeExec{}
	led := &fakeLedger{}

	res, err := Rollback(context.Background(), rollbackCfg(), "prod", ex, store, led, RollbackOptions{})
	require.NoError(t, err)
	require.Equal(t, "prev", res.ToSHA)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, store.committed)
	require.Equal(t, pin.Set{"SVC_IMAGE": "reg/svc:v1"}, ex.pins)
	last := led.entries[len(led.entries)-1]
	require.Equal(t, "rollback", last.By)
	require.Equal(t, "prod", last.Environment)
}

func TestRollback_NoHistory(t *testing.T) {
	store := &fakeStore{} // LatestCommit → ErrNoHistory
	_, err := Rollback(context.Background(), rollbackCfg(), "prod", &fakeExec{}, store, &fakeLedger{}, RollbackOptions{})
	require.ErrorContains(t, err, "no pin history")
}

func TestRollback_FirstCommit(t *testing.T) {
	store := &fakeStore{headSHA: "cur"} // ParentOf("cur") → ErrNoParent
	_, err := Rollback(context.Background(), rollbackCfg(), "prod", &fakeExec{}, store, &fakeLedger{}, RollbackOptions{})
	require.ErrorContains(t, err, "first pin commit")
}

func TestRollback_EmptyParentRefused(t *testing.T) {
	store := &fakeStore{
		headSHA: "cur",
		parent:  map[string]string{"cur": "prev"},
		// no atSHA["prev"] → ReadAt returns empty set
	}
	_, err := Rollback(context.Background(), rollbackCfg(), "prod", &fakeExec{}, store, &fakeLedger{}, RollbackOptions{})
	require.ErrorContains(t, err, "empty")
}

func TestRollback_DryRun(t *testing.T) {
	store := &fakeStore{
		headSHA: "cur",
		parent:  map[string]string{"cur": "prev"},
		atSHA:   map[string]pin.Set{"prev": {"SVC_IMAGE": "reg/svc:v1"}},
	}
	ex := &fakeExec{}
	res, err := Rollback(context.Background(), rollbackCfg(), "prod", ex, store, &fakeLedger{}, RollbackOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, res.DryRun)
	require.False(t, ex.called)
	require.Nil(t, store.committed)
}
