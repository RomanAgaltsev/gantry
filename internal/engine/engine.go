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
	"github.com/RomanAgaltsev/gantry/internal/logging"
	"github.com/RomanAgaltsev/gantry/internal/pin"
	"github.com/RomanAgaltsev/gantry/internal/verify"
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
	// Resolve expands a revision (a short SHA, full SHA, or ref) to a full commit SHA.
	Resolve(rev string) (sha string, err error)
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
// commit has a green ledger entry, redeploying if not.
func Sync(ctx context.Context, cfg *config.Config, envName string, f forge.Forge, ex executor.Executor, vf verify.Verifier, store PinStore, led ledger.Ledger, opts SyncOptions) (SyncResult, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return SyncResult{}, fmt.Errorf("environment %q not found", envName)
	}
	if env.Source.Track == "" {
		return SyncResult{}, fmt.Errorf("environment %q: sync supports track-mode only", envName)
	}

	log := logging.From(ctx)
	log.Info("polling forge", "env", envName, "components", len(cfg.Components))

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
		return ensureGreen(ctx, envName, env.PinFile, current, ex, nil, store, led, opts)
	}
	if opts.DryRun {
		return SyncResult{Changes: changes}, nil
	}

	sha, err := store.WriteAndCommit(env.PinFile, merged, commitMessage(envName, changes))
	if err != nil {
		return SyncResult{}, err
	}
	log.Info("pin written", "env", envName, "commit", sha, "changes", len(changes))

	if err := deployAndRecord(ctx, envName, env.PinFile, merged, sha, "sync", ex, nil, led); err != nil {
		return SyncResult{Changes: changes}, err
	}
	return SyncResult{Changes: changes, Deployed: true}, nil
}

// ensureGreen redeploys the current pins when the latest pin commit lacks a green
// ledger entry; otherwise it is a no-op. This makes a deploy that failed after its
// pin commit self-heal on the next Sync.
func ensureGreen(ctx context.Context, envName, pinFile string, current pin.Set, ex executor.Executor, _ verify.Verifier, store PinStore, led ledger.Ledger, opts SyncOptions) (SyncResult, error) {
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
	if err := deployAndRecord(ctx, envName, pinFile, current, sha, "sync", ex, nil, led); err != nil {
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
func Deploy(ctx context.Context, cfg *config.Config, envName string, ex executor.Executor, vf verify.Verifier, store PinStore, led ledger.Ledger) (DeployResult, error) {
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
	if err := deployAndRecord(ctx, envName, env.PinFile, pins, sha, "deploy", ex, nil, led); err != nil {
		return DeployResult{}, err
	}
	return DeployResult{Pins: pins, Deployed: true}, nil
}

// deployAndRecord deploys pins for env and records the outcome keyed by sha. A nil executor
// is a setup error. After a successful deploy it runs vf (when non-nil): a passing verify
// records healthy "true"; a failing one records result "failed", healthy "false", and is
// returned. With vf nil, healthy stays "unknown" (A2 behavior). A failed record is joined
// in so the self-heal signal the next Sync reads is never lost.
func deployAndRecord(ctx context.Context, env, pinFile string, pins pin.Set, sha, by string, ex executor.Executor, vf verify.Verifier, led ledger.Ledger) error {
	if ex == nil {
		return fmt.Errorf("no executor configured for environment %q", env)
	}
	_, deployErr := ex.Deploy(ctx, executor.Plan{Env: env, PinFile: pinFile, Pins: pins, Commit: sha})

	result, healthy := "ok", "unknown"
	var verifyErr error
	switch {
	case deployErr != nil:
		result = "failed"
	case vf != nil:
		if verifyErr = vf.Verify(ctx); verifyErr != nil {
			result, healthy = "failed", "false"
		} else {
			healthy = "true"
		}
	}

	logging.From(ctx).Info("deploy recorded", "env", env, "by", by, "result", result, "commit", sha)

	recErr := led.Record(ledger.Entry{
		Environment: env,
		PinCommit:   sha,
		Result:      result,
		Healthy:     healthy,
		ImageSet:    map[string]string(pins),
		DeployedAt:  time.Now(),
		By:          by,
	})

	actErr, verb := deployErr, "deploy"
	if actErr == nil && verifyErr != nil {
		actErr, verb = verifyErr, "verify"
	}
	if actErr != nil {
		if recErr != nil {
			return errors.Join(fmt.Errorf("%s %q: %w", verb, env, actErr),
				fmt.Errorf("record outcome: %w", recErr))
		}
		return fmt.Errorf("%s %q: %w", verb, env, actErr)
	}
	return recErr
}

// pinsEqual reports whether two pin sets hold exactly the same keys and values.
func pinsEqual(a, b pin.Set) bool {
	return len(a) == len(b) && len(pin.Diff(a, b)) == 0
}

// writePins commits pins to pinFile, returning the resulting commit SHA. When the file
// already holds exactly these pins it makes no commit (go-git rejects empty commits) and
// returns the existing latest commit instead, keeping a re-promote or a repeat rollback a
// redeploy rather than an empty commit.
func writePins(store PinStore, pinFile string, pins pin.Set, msg string) (string, error) {
	current, err := store.Read(pinFile)
	if err != nil {
		return "", err
	}
	if pinsEqual(current, pins) {
		return store.LatestCommit(pinFile)
	}
	return store.WriteAndCommit(pinFile, pins, msg)
}

// PromoteOptions tunes a Promote run.
type PromoteOptions struct{ DryRun bool }

// PromoteResult reports what a Promote did.
type PromoteResult struct {
	FromSHA   string  // the source commit snapshotted
	Pins      pin.Set // the promoted pin set
	Committed string  // the new commit on the target pin file
	Deployed  bool
	DryRun    bool
}

// Promote copies fromEnv's pin file as of sha into toEnv's pin file and deploys it.
// sha defaults to the latest green (Result=="ok") deploy of fromEnv; an explicit sha is
// gated — Promote refuses one whose (fromEnv, sha) ledger entry is missing or not ok.
// The snapshot is frozen: it never reads "the current upstream pin,"
// only the file as committed at sha.
func Promote(ctx context.Context, cfg *config.Config, fromEnv, toEnv, sha string, ex executor.Executor, vf verify.Verifier, store PinStore, led ledger.Ledger, opts PromoteOptions) (PromoteResult, error) {
	from, ok := cfg.Environment(fromEnv)
	if !ok {
		return PromoteResult{}, fmt.Errorf("environment %q not found", fromEnv)
	}
	to, ok := cfg.Environment(toEnv)
	if !ok {
		return PromoteResult{}, fmt.Errorf("environment %q not found", toEnv)
	}

	if sha == "" {
		var green ledger.Entry
		var err error
		if cfg.Promote.RequireHealthy {
			green, err = led.LatestHealthy(fromEnv)
		} else {
			green, err = led.LatestGreen(fromEnv)
		}
		if err != nil {
			return PromoteResult{}, fmt.Errorf("no %s deploy of %q to promote: %w", greenWord(cfg.Promote.RequireHealthy), fromEnv, err)
		}
		sha = green.PinCommit
	} else {
		full, err := store.Resolve(sha)
		if err != nil {
			return PromoteResult{}, fmt.Errorf("resolve --sha %q: %w", sha, err)
		}
		sha = full
		entry, ok, err := led.Lookup(fromEnv, sha)
		if err != nil {
			return PromoteResult{}, err
		}
		if !ok {
			return PromoteResult{}, fmt.Errorf("refusing to promote %q@%.7s: no deploy record (gate)", fromEnv, sha)
		}
		if entry.Result != "ok" {
			return PromoteResult{}, fmt.Errorf("refusing to promote %q@%.7s: last deploy was %q, not ok (gate)", fromEnv, sha, entry.Result)
		}
		if cfg.Promote.RequireHealthy && entry.Healthy != "true" {
			return PromoteResult{}, fmt.Errorf("refusing to promote %q@%.7s: not verified healthy (gate)", fromEnv, sha)
		}
	}

	pins, err := store.ReadAt(sha, from.PinFile)
	if err != nil {
		return PromoteResult{}, err
	}
	if len(pins) == 0 {
		return PromoteResult{}, fmt.Errorf("pin set at %.7s is empty; nothing to promote", sha)
	}
	if opts.DryRun {
		return PromoteResult{FromSHA: sha, Pins: pins, DryRun: true}, nil
	}

	msg := fmt.Sprintf("chore(%s): promote from %s@%.7s (%d pins)", toEnv, fromEnv, sha, len(pins))
	newSHA, err := writePins(store, to.PinFile, pins, msg)
	if err != nil {
		return PromoteResult{}, err
	}
	if err := deployAndRecord(ctx, toEnv, to.PinFile, pins, newSHA, "promote", ex, nil, led); err != nil {
		return PromoteResult{FromSHA: sha, Pins: pins, Committed: newSHA}, err
	}
	return PromoteResult{FromSHA: sha, Pins: pins, Committed: newSHA, Deployed: true}, nil
}

// greenWord labels the gate requirement for error messages.
func greenWord(requireHealthy bool) string {
	if requireHealthy {
		return "healthy"
	}
	return "green"
}

// RollbackOptions tunes a Rollback run.
type RollbackOptions struct{ DryRun bool }

// RollbackResult reports what a Rollback did.
type RollbackResult struct {
	ToSHA     string  // the green pin commit whose set was restored
	Pins      pin.Set // the restored pin set
	Committed string  // the new commit recording the rollback (or the existing one if unchanged)
	Deployed  bool
	DryRun    bool
	Slot      string // blue-green: the slot now live after the flip-back ("" for file-model)
}

// Rollback restores env to the most recent GREEN deploy older than its current pin commit,
// commits that set, deploys, and records the outcome. Targeting the last known-good ledger
// entry (rather than the literal parent commit) means rollback never redeploys a set the
// ledger knows failed, and repeated rollbacks walk backward through good states instead of
// oscillating onto the bad one. Immutable image tags keep the previous images addressable.
func Rollback(ctx context.Context, cfg *config.Config, envName string, ex executor.Executor, vf verify.Verifier, store PinStore, led ledger.Ledger, opts RollbackOptions) (RollbackResult, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return RollbackResult{}, fmt.Errorf("environment %q not found", envName)
	}
	if se, ok := ex.(executor.SlotExecutor); ok {
		return slotRollback(ctx, envName, env.PinFile, se, store, led, opts)
	}
	last, err := store.LatestCommit(env.PinFile)
	if errors.Is(err, ErrNoHistory) {
		return RollbackResult{}, fmt.Errorf("environment %q has no pin history to roll back", envName)
	}
	if err != nil {
		return RollbackResult{}, err
	}
	hist, err := led.History(envName)
	if err != nil {
		return RollbackResult{}, err
	}
	target := ""
	for _, e := range hist {
		if e.Result == "ok" && e.PinCommit != last {
			target = e.PinCommit
			break
		}
	}
	if target == "" {
		return RollbackResult{}, fmt.Errorf("no earlier green deploy of %q to roll back to", envName)
	}
	pins, err := store.ReadAt(target, env.PinFile)
	if err != nil {
		return RollbackResult{}, err
	}
	if len(pins) == 0 {
		return RollbackResult{}, fmt.Errorf("pin set at %.7s is empty; refusing to roll back %q to an empty stack", target, envName)
	}
	if opts.DryRun {
		return RollbackResult{ToSHA: target, Pins: pins, DryRun: true}, nil
	}

	msg := fmt.Sprintf("chore(%s): rollback to %.7s (%d pins)", envName, target, len(pins))
	newSHA, err := writePins(store, env.PinFile, pins, msg)
	if err != nil {
		return RollbackResult{ToSHA: target, Pins: pins}, err
	}
	if err := deployAndRecord(ctx, envName, env.PinFile, pins, newSHA, "rollback", ex, nil, led); err != nil {
		return RollbackResult{ToSHA: target, Pins: pins, Committed: newSHA}, err
	}
	return RollbackResult{ToSHA: target, Pins: pins, Committed: newSHA, Deployed: true}, nil
}

// slotRollback rolls a blue-green environment back by flipping the pointer to the other
// (previously-live) slot, which still runs the prior version.
func slotRollback(ctx context.Context, envName, pinFile string, se executor.SlotExecutor, store PinStore, led ledger.Ledger, opts RollbackOptions) (RollbackResult, error) {
	live, err := se.LiveSlot(ctx)
	if err != nil {
		return RollbackResult{}, err
	}
	if live == "" {
		return RollbackResult{}, fmt.Errorf("environment %q has no live slot to roll back from", envName)
	}
	a, b := se.Slots()
	target := otherSlot(a, b, live)
	if opts.DryRun {
		return RollbackResult{Slot: target, DryRun: true}, nil
	}
	if err := se.SwitchTo(ctx, target); err != nil {
		return RollbackResult{Slot: target}, err
	}
	head, err := store.LatestCommit(pinFile)
	if err != nil && !errors.Is(err, ErrNoHistory) {
		return RollbackResult{Slot: target, Deployed: true}, err
	}
	rec := ledger.Entry{
		Environment: envName,
		PinCommit:   head,
		Result:      "ok",
		DeployedAt:  time.Now(),
		By:          "rollback",
	}
	if err := led.Record(rec); err != nil {
		return RollbackResult{Slot: target, Deployed: true}, err
	}
	return RollbackResult{Slot: target, Deployed: true}, nil
}

// SwitchResult reports a blue-green pointer switch.
type SwitchResult struct {
	From      string // live slot before the switch ("" at bootstrap)
	To        string // live slot after the switch
	Committed string // pin commit the switch promoted (the head)
}

// otherSlot returns the slot that is not live; an unset/unknown live slot resolves to a.
func otherSlot(a, b, live string) string {
	if live == a {
		return b
	}
	if live == b {
		return a
	}
	return a
}

// Switch promotes the idle slot of a blue-green environment by flipping its pointer, gated
// on the environment's current head pin commit having an ok ledger entry. It requires an
// executor implementing executor.SlotExecutor.
func Switch(ctx context.Context, cfg *config.Config, envName string, ex executor.Executor, store PinStore, led ledger.Ledger) (SwitchResult, error) {
	env, ok := cfg.Environment(envName)
	if !ok {
		return SwitchResult{}, fmt.Errorf("environment %q not found", envName)
	}
	se, ok := ex.(executor.SlotExecutor)
	if !ok {
		return SwitchResult{}, fmt.Errorf("environment %q is not a blue-green environment", envName)
	}
	head, err := store.LatestCommit(env.PinFile)
	if errors.Is(err, ErrNoHistory) {
		return SwitchResult{}, fmt.Errorf("environment %q has nothing staged to switch", envName)
	}
	if err != nil {
		return SwitchResult{}, err
	}
	entry, ok, err := led.Lookup(envName, head)
	if err != nil {
		return SwitchResult{}, err
	}
	if !ok || entry.Result != "ok" {
		return SwitchResult{}, fmt.Errorf("refusing to switch %q: idle slot deploy at %.7s is not ok (gate)", envName, head)
	}
	live, err := se.LiveSlot(ctx)
	if err != nil {
		return SwitchResult{}, err
	}
	a, b := se.Slots()
	idle := otherSlot(a, b, live)
	if err := se.SwitchTo(ctx, idle); err != nil {
		return SwitchResult{From: live, To: idle, Committed: head}, err
	}
	rec := ledger.Entry{
		Environment: envName,
		PinCommit:   head,
		Result:      "ok",
		Healthy:     entry.Healthy,
		ImageSet:    entry.ImageSet,
		DeployedAt:  time.Now(),
		By:          "switch",
	}
	if err := led.Record(rec); err != nil {
		return SwitchResult{From: live, To: idle, Committed: head}, err
	}
	return SwitchResult{From: live, To: idle, Committed: head}, nil
}
