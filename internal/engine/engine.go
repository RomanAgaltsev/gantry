// Package engine orchestrates the consume → pin → deploy flow.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
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
	Changes   []pin.Change
	Deployed  bool
	Recovered bool // a previously committed-but-undeployed pin set was redeployed
}

// Sync resolves each component's latest release into the environment's pin file
// (commit-on-diff) and deploys via ex. With no diff it still ensures the latest pin
// commit has a green ledger entry, redeploying if not (decision A2-D7).
func Sync(ctx context.Context, cfg *config.Config, envName string, f forge.Forge, ex executor.Executor, store PinStore, led ledger.Ledger, opts SyncOptions) (SyncResult, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return SyncResult{}, fmt.Errorf("environment %q not found", envName)
	}
	if env.Source.Track == "" {
		return SyncResult{}, fmt.Errorf("environment %q: sync supports track-mode only", envName)
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
	merged := pin.Set{}
	for k, v := range current {
		merged[k] = v
	}
	for k, v := range desired {
		merged[k] = v
	}

	changes := pin.Diff(current, merged)
	if len(changes) == 0 {
		return ensureGreen(ctx, envName, env.PinFile, current, ex, store, led, opts)
	}
	if opts.DryRun {
		return SyncResult{Changes: changes}, nil
	}

	sha, err := store.WriteAndCommit(env.PinFile, merged, commitMessage(envName, changes))
	if err != nil {
		return SyncResult{}, err
	}
	if err := deployAndRecord(ctx, envName, env.PinFile, merged, sha, "sync", ex, led); err != nil {
		return SyncResult{Changes: changes}, err
	}
	return SyncResult{Changes: changes, Deployed: true}, nil
}

// ensureGreen redeploys the current pins when the latest pin commit lacks a green
// ledger entry; otherwise it is a no-op. This makes a deploy that failed after its
// pin commit self-heal on the next Sync.
func ensureGreen(ctx context.Context, envName, pinFile string, current pin.Set, ex executor.Executor, store PinStore, led ledger.Ledger, opts SyncOptions) (SyncResult, error) {
	sha, err := store.LatestCommit(pinFile)
	if errors.Is(err, ErrNoHistory) {
		return SyncResult{}, nil // nothing was ever committed; nothing to ensure
	}
	if err != nil {
		return SyncResult{}, err
	}
	entry, ok, err := led.Lookup(envName, sha)
	if err != nil {
		return SyncResult{}, err
	}
	if ok && entry.Result == "ok" {
		return SyncResult{}, nil // already green
	}
	if opts.DryRun {
		return SyncResult{Recovered: true}, nil
	}
	if err := deployAndRecord(ctx, envName, pinFile, current, sha, "sync", ex, led); err != nil {
		return SyncResult{Recovered: true}, err
	}
	return SyncResult{Deployed: true, Recovered: true}, nil
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

// Deploy reconciles the running stack of an environment to its current committed pin
// file (the path CI runs on a Renovate/explicit bump or a promotion commit) and records
// the outcome.
func Deploy(ctx context.Context, cfg *config.Config, envName string, ex executor.Executor, store PinStore, led ledger.Ledger) (DeployResult, error) {
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
	sha, err := store.LatestCommit(env.PinFile)
	if err != nil {
		return DeployResult{}, err
	}
	if err := deployAndRecord(ctx, envName, env.PinFile, pins, sha, "deploy", ex, led); err != nil {
		return DeployResult{}, err
	}
	return DeployResult{Pins: pins, Deployed: true}, nil
}

// deployAndRecord deploys pins for env and records the outcome keyed by sha. A nil
// executor is a setup error (no record is written); a deploy failure is recorded as
// "failed" and returned, so the next Sync can self-heal (decision A2-D7).
func deployAndRecord(ctx context.Context, env, pinFile string, pins pin.Set, sha, by string, ex executor.Executor, led ledger.Ledger) error {
	if ex == nil {
		return fmt.Errorf("no executor configured for environment %q", env)
	}
	_, deployErr := ex.Deploy(ctx, executor.Plan{Env: env, PinFile: pinFile, Pins: pins})
	result := "ok"
	if deployErr != nil {
		result = "failed"
	}
	recErr := led.Record(ledger.Entry{
		Environment: env,
		PinCommit:   sha,
		Result:      result,
		Healthy:     "unknown",
		ImageSet:    map[string]string(pins),
		DeployedAt:  time.Now(),
		By:          by,
	})
	if deployErr != nil {
		return fmt.Errorf("deploy %q: %w", env, deployErr)
	}
	return recErr
}
