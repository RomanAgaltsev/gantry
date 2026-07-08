package engine

import (
	"context"
	"sync"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/logging"
)

// untrackedRef is the Latest value shown for explicit (registry-sourced) components,
// which have no gantry-known "latest" release.
const untrackedRef = "(untracked)"

// errorRef is the Latest value shown for a component whose release fetch failed, so one bad
// component (a repo without a release, a 404) degrades its cell instead of failing the whole
// matrix (C5). An error cell never counts as drift.
const errorRef = "(error)"

// statusFetchConcurrency bounds concurrent forge calls when building the matrix, so a wide
// component list overlaps latency without opening an unbounded number of connections (P1).
const statusFetchConcurrency = 8

// EnvHealth is one environment's most recent deploy outcome from the ledger.
type EnvHealth struct {
	Env     string        `json:"env"`
	Result  string        `json:"result"`   // ledger Result ("ok"|"failed"); "" when HasData is false
	Healthy string        `json:"healthy"`  // ledger Healthy ("true"|"false"|"unknown"); "" when HasData is false
	Age     time.Duration `json:"age"`      // now − newest entry's DeployedAt; 0 when HasData is false
	HasData bool          `json:"has_data"` // false when the environment has no ledger history yet
}

// Matrix is the cross-environment read model behind `gantry status --all`:
// the latest release per component, each environment's pins, which cells lag
// latest, and each environment's health. Computed live; nothing is stored.
type Matrix struct {
	Components   []string                     `json:"components"`   // pin keys, config order
	Environments []string                     `json:"environments"` // env names, config order
	Latest       map[string]string            `json:"latest"`       // pinKey -> latest ref or "(untracked)"
	Pins         map[string]map[string]string `json:"pins"`         // env -> pinKey -> pinned ref ("" if absent)
	Drift        map[string]map[string]bool   `json:"drift"`        // env -> pinKey -> pin lags latest (tracked only)
	Health       []EnvHealth                  `json:"health"`       // per environment, Environments order
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

	// Latest per component (once, C1-D6). Fetches run in a bounded worker pool (P1) so a wide
	// component list overlaps forge latency; results are collected by index and assembled in
	// config order so the matrix output is byte-identical to the serial version.
	type latestResult struct {
		pinKey string
		ref    string
	}
	results := make([]latestResult, len(e.Cfg.Components))
	sem := make(chan struct{}, statusFetchConcurrency)
	var wg sync.WaitGroup
	for i, comp := range e.Cfg.Components {
		results[i].pinKey = comp.PinKey
		if comp.IsExplicit() {
			results[i].ref = untrackedRef
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, comp config.Component) {
			defer wg.Done()
			defer func() { <-sem }()
			rel, err := e.Forge.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
			if err != nil {
				// One component's forge error degrades its cell instead of failing the whole matrix
				// — you most need status during an incident (C5). Preserve that under parallelism.
				logging.From(ctx).Warn("status: latest release unavailable", "component", comp.ID, "error", err)
				results[i].ref = errorRef
				return
			}
			results[i].ref = rel.ImageRef()
		}(i, comp)
	}
	wg.Wait()
	for _, r := range results {
		m.Components = append(m.Components, r.pinKey)
		m.Latest[r.pinKey] = r.ref
	}

	// Pins + drift + health per environment.
	now := timeNow()
	for _, env := range e.Cfg.Environments {
		m.Environments = append(m.Environments, env.Name)

		pins, err := e.Store.Read(ctx, env.PinFile)
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

		hist, err := e.Ledger.History(ctx, env.Name)
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
