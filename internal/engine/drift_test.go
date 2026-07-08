package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// driftForge returns a per-component release keyed by component ID.
type driftForge struct{ byID map[string]forge.Release }

func (f driftForge) LatestRelease(_ context.Context, c forge.Component) (forge.Release, error) {
	r := f.byID[c.ID]
	r.Component = c.ID
	return r, nil
}

func driftCfg(threshold config.Duration, comps ...config.Component) *config.Config {
	return &config.Config{
		Components: comps,
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
			{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
		},
		Drift: config.DriftConfig{Threshold: threshold},
	}
}

func rel(repo, tag string, builtAt time.Time) forge.Release {
	return forge.Release{ImageRepository: repo, ImageTag: tag, SemverVersion: tag, BuiltAt: builtAt}
}

func TestDrift(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	t.Cleanup(func() { timeNow = time.Now })

	stale := fixedNow.Add(-9 * 24 * time.Hour) // 9 days old
	fresh := fixedNow.Add(-1 * time.Hour)      // 1 hour old
	threshold := config.Duration(7 * 24 * time.Hour)

	t.Run("no drift when pin matches latest", func(t *testing.T) {
		cfg := driftCfg(threshold, config.Component{ID: "api", Project: "demo/api", PinKey: "API_IMAGE"})
		store := &fakeStore{cur: pin.Set{"API_IMAGE": "reg/api:v2"}}
		f := driftForge{byID: map[string]forge.Release{"api": rel("reg/api", "v2", stale)}}
		rep, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "test")
		require.NoError(t, err)
		require.False(t, rep.Drifted())
	})

	t.Run("no drift when newer but fresh (within threshold)", func(t *testing.T) {
		cfg := driftCfg(threshold, config.Component{ID: "api", Project: "demo/api", PinKey: "API_IMAGE"})
		store := &fakeStore{cur: pin.Set{"API_IMAGE": "reg/api:v1"}}
		f := driftForge{byID: map[string]forge.Release{"api": rel("reg/api", "v2", fresh)}}
		rep, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "test")
		require.NoError(t, err)
		require.False(t, rep.Drifted())
	})

	t.Run("drift when newer and stale", func(t *testing.T) {
		cfg := driftCfg(threshold, config.Component{ID: "api", Project: "demo/api", PinKey: "API_IMAGE"})
		store := &fakeStore{cur: pin.Set{"API_IMAGE": "reg/api:v1"}}
		f := driftForge{byID: map[string]forge.Release{"api": rel("reg/api", "v2", stale)}}
		rep, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "test")
		require.NoError(t, err)
		require.True(t, rep.Drifted())
		require.Len(t, rep.Items, 1)
		require.Equal(t, "api", rep.Items[0].Component)
		require.Equal(t, "reg/api:v1", rep.Items[0].PinnedRef)
		require.Equal(t, "reg/api:v2", rep.Items[0].LatestRef)
		require.Equal(t, 9*24*time.Hour, rep.Items[0].Age)
	})

	t.Run("explicit-pin component is skipped even when stale", func(t *testing.T) {
		cfg := driftCfg(threshold, config.Component{ID: "pg", PinKey: "PG_IMAGE", Source: config.ComponentSource{Pin: "explicit"}})
		store := &fakeStore{cur: pin.Set{"PG_IMAGE": "postgres:15"}}
		f := driftForge{byID: map[string]forge.Release{}} // never consulted
		rep, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "test")
		require.NoError(t, err)
		require.False(t, rep.Drifted())
	})

	t.Run("never-pinned component is drift once stale", func(t *testing.T) {
		cfg := driftCfg(threshold, config.Component{ID: "api", Project: "demo/api", PinKey: "API_IMAGE"})
		store := &fakeStore{cur: pin.Set{}} // nothing pinned yet
		f := driftForge{byID: map[string]forge.Release{"api": rel("reg/api", "v1", stale)}}
		rep, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "test")
		require.NoError(t, err)
		require.True(t, rep.Drifted())
		require.Equal(t, "", rep.Items[0].PinnedRef)
	})

	t.Run("non-track env errors", func(t *testing.T) {
		cfg := driftCfg(threshold, config.Component{ID: "api", Project: "demo/api", PinKey: "API_IMAGE"})
		store := &fakeStore{cur: pin.Set{}}
		f := driftForge{byID: map[string]forge.Release{}}
		_, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "prod")
		require.Error(t, err)
		require.Contains(t, err.Error(), "track-mode")
	})
}

// TestDrift_PerComponentThreshold checks that a component's own drift_threshold overrides
// the global one: a 3-day-old release is within the 7d global threshold but past the
// component's 1d threshold, so it drifts (review §9.12).
func TestDrift_PerComponentThreshold(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	t.Cleanup(func() { timeNow = time.Now })

	age3d := fixedNow.Add(-3 * 24 * time.Hour)
	cfg := driftCfg(config.Duration(7*24*time.Hour),
		config.Component{ID: "api", Project: "demo/api", PinKey: "API_IMAGE", DriftThreshold: config.Duration(24 * time.Hour)})
	store := &fakeStore{cur: pin.Set{"API_IMAGE": "reg/api:v1"}}
	f := driftForge{byID: map[string]forge.Release{"api": rel("reg/api", "v2", age3d)}}

	rep, err := (&Engine{Cfg: cfg, Forge: f, Store: store}).Drift(context.Background(), "test")
	require.NoError(t, err)
	require.True(t, rep.Drifted())
}
