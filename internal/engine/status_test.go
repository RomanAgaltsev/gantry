package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func TestStatusMatrix_GridDriftHealth(t *testing.T) {
	fixed := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	orig := timeNow
	timeNow = func() time.Time { return fixed }
	defer func() { timeNow = orig }()

	cfg := &config.Config{
		Components: []config.Component{
			{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
			{ID: "pg", PinKey: "PG_IMAGE", Source: config.ComponentSource{Pin: "explicit"}},
		},
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
			{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
		},
	}
	f := fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v9"}}
	store := &fakeStore{byFile: map[string]pin.Set{
		".env.versions.test": {"SVC_IMAGE": "reg/svc:v9", "PG_IMAGE": "postgres:16.2"},
		".env.versions.prod": {"SVC_IMAGE": "reg/svc:v8", "PG_IMAGE": "postgres:16.2"},
	}}
	led := &fakeLedger{entries: []ledger.Entry{
		{Environment: "test", Result: "ok", Healthy: "true", DeployedAt: fixed.Add(-2 * time.Hour)},
	}}

	m, err := (&Engine{Cfg: cfg, Forge: f, Store: store, Ledger: led}).StatusMatrix(context.Background())
	require.NoError(t, err)

	require.Equal(t, []string{"SVC_IMAGE", "PG_IMAGE"}, m.Components)
	require.Equal(t, []string{"test", "prod"}, m.Environments)

	require.Equal(t, "reg/svc:v9", m.Latest["SVC_IMAGE"])
	require.Equal(t, "(untracked)", m.Latest["PG_IMAGE"]) // explicit component

	require.Equal(t, "reg/svc:v9", m.Pins["test"]["SVC_IMAGE"])
	require.Equal(t, "reg/svc:v8", m.Pins["prod"]["SVC_IMAGE"])

	require.False(t, m.Drift["test"]["SVC_IMAGE"]) // up to date
	require.True(t, m.Drift["prod"]["SVC_IMAGE"])  // lags latest
	require.False(t, m.Drift["test"]["PG_IMAGE"])  // explicit never drifts

	require.Len(t, m.Health, 2)
	require.Equal(t, "test", m.Health[0].Env)
	require.True(t, m.Health[0].HasData)
	require.Equal(t, "ok", m.Health[0].Result)
	require.Equal(t, "true", m.Health[0].Healthy)
	require.Equal(t, 2*time.Hour, m.Health[0].Age)
	require.False(t, m.Health[1].HasData) // prod has no ledger entry
}

func TestStatusMatrix_DegradesPerCellOnForgeError(t *testing.T) {
	ok := forge.Release{ImageRepository: "reg/pg", ImageTag: "v9", BuiltAt: time.Now()}
	cfg := &config.Config{
		Components: []config.Component{
			{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
			{ID: "pg", Project: "g/pg", PinKey: "PG_IMAGE", Source: config.ComponentSource{Forge: "release"}},
		},
		Environments: []config.Environment{{Name: "test", Source: config.Source{Track: "latest"}, PinFile: "f"}},
	}
	store := &fakeStore{byFile: map[string]pin.Set{"f": {"SVC_IMAGE": "reg/svc:v1", "PG_IMAGE": "reg/pg:v9"}}}
	m, err := (&Engine{Cfg: cfg, Forge: perIDErrForge{failID: "svc", rel: ok}, Store: store, Ledger: &fakeLedger{}}).StatusMatrix(context.Background())
	require.NoError(t, err, "one bad component must not fail the whole matrix (C5)")
	require.Equal(t, "(error)", m.Latest["SVC_IMAGE"], "failing cell degrades to the (error) sentinel")
	require.Equal(t, ok.ImageRef(), m.Latest["PG_IMAGE"], "healthy component still resolves")
	require.False(t, m.Drift["test"]["SVC_IMAGE"], "an error cell never counts as drift")
}

// perIDErrForge fails LatestRelease for one component ID and succeeds (fixed release) for others.
type perIDErrForge struct {
	failID string
	rel    forge.Release
}

func (f perIDErrForge) LatestRelease(_ context.Context, c forge.Component) (forge.Release, error) {
	if c.ID == f.failID {
		return forge.Release{}, errForgeFail
	}
	r := f.rel
	r.Component = c.ID
	return r, nil
}

var errForgeFail = testError("forge down")

type testError string

func (e testError) Error() string { return string(e) }
