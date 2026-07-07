package engine

import (
	"context"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/logging"
)

// NOTE: the package var `timeNow = time.Now` is ALREADY declared in this package
// by the (already-implemented) A3 drift detector — `internal/engine/drift.go`.
// StatusMatrix reuses it; do NOT redeclare it here (a duplicate declaration will
// not compile). The status_test.go override below shares that same var.

// untrackedRef is the Latest value shown for explicit (registry-sourced) components,
// which have no gantry-known "latest" release.
const untrackedRef = "(untracked)"

// errorRef is the Latest value shown for a component whose release fetch failed, so one bad
// component (a repo without a release, a 404) degrades its cell instead of failing the whole
// matrix (C5). An error cell never counts as drift.
const errorRef = "(error)"

// EnvHealth is one environment's most recent deploy outcome from the ledger.
type EnvHealth struct {
	Env     string
	Result  string        // ledger Result ("ok"|"failed"); "" when HasData is false
	Healthy string        // ledger Healthy ("true"|"false"|"unknown"); "" when HasData is false
	Age     time.Duration // now − newest entry's DeployedAt; 0 when HasData is false
	HasData bool          // false when the environment has no ledger history yet
}

// Matrix is the cross-environment read model behind `gantry status --all`:
// the latest release per component, each environment's pins, which cells lag
// latest, and each environment's health. Computed live; nothing is stored.
type Matrix struct {
	Components   []string                     // pin keys, config order
	Environments []string                     // env names, config order
	Latest       map[string]string            // pinKey -> latest ref or "(untracked)"
	Pins         map[string]map[string]string // env -> pinKey -> pinned ref ("" if absent)
	Drift        map[string]map[string]bool   // env -> pinKey -> pin lags latest (tracked only)
	Health       []EnvHealth                  // per environment, Environments order
}

// StatusMatrix builds the status matrix. It fetches each component's latest
// release once (a property of the component, not the environment), reads every
// environment's pin file, and reads each environment's newest ledger entry.
// Read-only: no commit, no deploy.
func (e *Engine) StatusMatrix(ctx context.Context) (Matrix, error) {
	m := Matrix{
		Latest: map[string]string{},
		Pins:   map[string]map[string]string{},
		Drift:  map[string]map[string]bool{},
	}

	// Latest per component (once, C1-D6).
	for _, comp := range e.Cfg.Components {
		m.Components = append(m.Components, comp.PinKey)
		if comp.IsExplicit() {
			m.Latest[comp.PinKey] = untrackedRef
			continue
		}
		rel, err := e.Forge.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
		if err != nil {
			// One component's forge error (a repo without a release, a 404) degrades this cell
			// instead of failing the whole matrix — you most need status during an incident (C5).
			logging.From(ctx).Warn("status: latest release unavailable", "component", comp.ID, "error", err)
			m.Latest[comp.PinKey] = errorRef
			continue
		}
		m.Latest[comp.PinKey] = rel.ImageRef()
	}

	// Pins + drift + health per environment.
	now := timeNow()
	for _, env := range e.Cfg.Environments {
		m.Environments = append(m.Environments, env.Name)

		pins, err := e.Store.Read(env.PinFile)
		if err != nil {
			return Matrix{}, err
		}
		cells := map[string]string{}
		drift := map[string]bool{}
		for _, comp := range e.Cfg.Components {
			ref := pins[comp.PinKey]
			cells[comp.PinKey] = ref
			// An error cell (release fetch failed) is excluded from drift: "latest unavailable"
			// must not masquerade as "pinned behind latest" (C5).
			drift[comp.PinKey] = !comp.IsExplicit() && ref != "" &&
				m.Latest[comp.PinKey] != errorRef && ref != m.Latest[comp.PinKey]
		}
		m.Pins[env.Name] = cells
		m.Drift[env.Name] = drift

		hist, err := e.Ledger.History(env.Name)
		if err != nil {
			return Matrix{}, err
		}
		h := EnvHealth{Env: env.Name}
		if len(hist) > 0 {
			entry := hist[0] // newest first
			h.Result, h.Healthy, h.Age, h.HasData = string(entry.Result), string(entry.Healthy), now.Sub(entry.DeployedAt), true
		}
		m.Health = append(m.Health, h)
	}

	return m, nil
}
