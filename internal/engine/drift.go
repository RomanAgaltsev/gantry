package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
)

// timeNow is the clock Drift compares release ages against; overridable in tests.
var timeNow = time.Now

// DriftItem is one tracked component whose latest release is newer than its pin and
// has been published longer than the threshold.
type DriftItem struct {
	Env       string
	Component string
	PinKey    string
	PinnedRef string // current pin ("" if never pinned)
	LatestRef string // latest release's ImageRef
	Latest    forge.Release
	Age       time.Duration // now − Latest.BuiltAt
}

// DriftReport lists every drifted component for an environment.
type DriftReport struct{ Items []DriftItem }

// Drifted reports whether any component has drifted.
func (r DriftReport) Drifted() bool { return len(r.Items) > 0 }

// Drift reports tracked components whose latest published release differs from the
// current pin and has been published longer than cfg.Drift threshold. Read-only:
// it never writes, commits, or deploys. It errors when env is not track-mode.
func Drift(ctx context.Context, cfg *config.Config, envName string, f forge.Forge, store PinStore) (DriftReport, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return DriftReport{}, fmt.Errorf("environment %q not found", envName)
	}
	if env.Source.Track == "" {
		return DriftReport{}, fmt.Errorf("environment %q is not track-mode; drift applies to track-mode environments only", envName)
	}
	current, err := store.Read(env.PinFile)
	if err != nil {
		return DriftReport{}, err
	}
	threshold := cfg.Drift.ThresholdOrDefault()
	now := timeNow()

	var rep DriftReport
	for _, comp := range cfg.Components {
		if comp.IsExplicit() {
			continue // explicit pins have no gantry-known "latest" (B4-D3)
		}
		rel, err := f.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
		if err != nil {
			return DriftReport{}, err
		}
		latestRef := rel.ImageRef()
		if latestRef == current[comp.PinKey] {
			continue // already pinned to the latest
		}
		age := now.Sub(rel.BuiltAt)
		if age <= threshold {
			continue // newer release exists but is still within tolerance
		}
		rep.Items = append(rep.Items, DriftItem{
			Env:       envName,
			Component: comp.ID,
			PinKey:    comp.PinKey,
			PinnedRef: current[comp.PinKey],
			LatestRef: latestRef,
			Latest:    rel,
			Age:       age,
		})
	}
	return rep, nil
}
