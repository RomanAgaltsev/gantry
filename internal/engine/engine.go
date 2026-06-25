// Package engine orchestrates the consume → pin → deploy flow.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// ErrNoHistory is returned when a pin file has no commit touching it.
var ErrNoHistory = errors.New("pin file has no commit history")

// ErrNoParent is returned when a commit has no parent (the first commit).
var ErrNoParent = errors.New("commit has no parent")

// PinStore reads and commits an environment's pin file.
type PinStore interface {
	Read(pinFile string) (pin.Set, error)
	ReadAt(sha, pinFile string) (pin.Set, error)
	WriteAndCommit(pinFile string, s pin.Set, msg string) (sha string, err error)
	LatestCommit(pinFile string) (sha string, err error)
	ParentOf(sha string) (parent string, err error)
}

// SyncOptions tunes a Sync run.
type SyncOptions struct{ DryRun bool }

// SyncResult reports what a Sync did.
type SyncResult struct {
	Changes  []pin.Change
	Deployed bool
}

// Sync resolves each component's latest release into the environment's pin file
// (commit-on-diff) and deploys via ex when the pins changed.
func Sync(ctx context.Context, cfg *config.Config, envName string, f forge.Forge, ex executor.Executor, store PinStore, opts SyncOptions) (SyncResult, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return SyncResult{}, fmt.Errorf("environment %q not found", envName)
	}
	if env.Source.Track == "" {
		return SyncResult{}, fmt.Errorf("environment %q: slice 1 supports track-mode only", envName)
	}

	desired := pin.Set{}
	for _, comp := range cfg.Components {
		if comp.IsExplicit() {
			continue // pin maintained in the pin file; never polled/overwritten (B4-D3)
		}
		rel, err := f.LatestRelease(ctx, forge.Component{ID: comp.ID, Project: comp.Project, PinKey: comp.PinKey})
		if err != nil {
			return SyncResult{}, err
		}
		desired[comp.PinKey] = rel.ImageRef()
	}

	current, err := store.Read(env.PinFile)
	if err != nil {
		return SyncResult{}, err
	}
	// Merge unknown (non-component) keys forward so we never drop them.
	merged := pin.Set{}
	for k, v := range current {
		merged[k] = v
	}
	for k, v := range desired {
		merged[k] = v
	}

	changes := pin.Diff(current, merged)
	if len(changes) == 0 {
		return SyncResult{}, nil
	}
	if opts.DryRun {
		return SyncResult{Changes: changes}, nil
	}

	if ex == nil {
		return SyncResult{}, fmt.Errorf("no executor configured for environment %q", envName)
	}

	msg := commitMessage(envName, changes)
	if _, err := store.WriteAndCommit(env.PinFile, merged, msg); err != nil {
		return SyncResult{}, err
	}
	if _, err := ex.Deploy(ctx, executor.Plan{Env: envName, PinFile: env.PinFile, Pins: merged}); err != nil {
		// The new pins are already committed but not deployed: a plain re-`sync`
		// sees no diff and won't retry. Make the drift recoverable by hand.
		return SyncResult{Changes: changes}, fmt.Errorf(
			"deploy failed after committing pins for %q; run `gantry deploy --env %s` to retry: %w",
			envName, envName, err,
		)
	}
	return SyncResult{Changes: changes, Deployed: true}, nil
}

func commitMessage(env string, changes []pin.Change) string {
	var b strings.Builder
	fmt.Fprintf(&b, "chore(%s): pin %d component(s)\n\n", env, len(changes))
	for _, c := range changes {
		fmt.Fprintf(&b, "%s: %s -> %s\n", c.Key, c.Old, c.New)
	}
	return b.String()
}

// DeployResult reports what a Deploy did.
type DeployResult struct {
	Pins     pin.Set
	Deployed bool
}

// Deploy reconciles the running stack of an environment to its current committed
// pin file, regardless of how each pin got its value (forge-derived or explicit).
// This is the path CI runs when the pin file changes (a Renovate or explicit bump,
// or a promotion commit).
func Deploy(ctx context.Context, cfg *config.Config, envName string, ex executor.Executor, store PinStore) (DeployResult, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return DeployResult{}, fmt.Errorf("environment %q not found", envName)
	}
	pins, err := store.Read(env.PinFile)
	if err != nil {
		return DeployResult{}, err
	}
	if len(pins) == 0 {
		return DeployResult{}, fmt.Errorf("pin file %q is empty; nothing to deploy", env.PinFile)
	}
	if ex == nil {
		return DeployResult{}, fmt.Errorf("no executor configured for environment %q", envName)
	}
	if _, err := ex.Deploy(ctx, executor.Plan{Env: envName, PinFile: env.PinFile, Pins: pins}); err != nil {
		return DeployResult{}, err
	}
	return DeployResult{Pins: pins, Deployed: true}, nil
}
