package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/forge"
)

// timeNow is the clock Drift compares release ages against; overridable in tests.
var timeNow = time.Now

// DriftItem is one tracked component whose latest release is newer than its pin and
// has been published longer than the threshold.
type DriftItem struct {
	Env       string        `json:"env"`
	Component string        `json:"component"`
	PinKey    string        `json:"pin_key"`
	PinnedRef string        `json:"pinned_ref"` // current pin ("" if never pinned)
	LatestRef string        `json:"latest_ref"` // latest release's ImageRef
	Latest    forge.Release `json:"latest"`
	Age       time.Duration `json:"age"` // now − Latest.BuiltAt
}

// DriftReport lists every drifted component for an environment.
type DriftReport struct{ Items []DriftItem }

// Drifted reports whether any component has drifted.
func (r DriftReport) Drifted() bool { return len(r.Items) > 0 }

// Drift reports tracked components whose latest published release differs from the
// current pin and has been published longer than cfg.Drift threshold. Read-only:
// it never writes, commits, or deploys. It errors when env is not track-mode.
func (e *Engine) Drift(ctx context.Context, envName string) (DriftReport, error) {
	env, ok := e.Cfg.Environment(envName)
	if !ok {
		return DriftReport{}, fmt.Errorf("environment %q not found", envName)
	}
	if env.Source.Track == "" {
		return DriftReport{}, fmt.Errorf("environment %q is not track-mode; drift applies to track-mode environments only", envName)
	}
	current, err := e.Store.Read(ctx, env.PinFile)
	if err != nil {
		return DriftReport{}, err
	}
	threshold := e.Cfg.Drift.ThresholdOrDefault()
	now := timeNow()

	var rep DriftReport
	for _, comp := range e.Cfg.Components {
		if comp.IsExplicit() {
			continue // explicit pins have no gantry-known "latest" (B4-D3)
		}
		rel, err := e.Forge.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
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
