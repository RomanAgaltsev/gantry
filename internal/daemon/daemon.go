package daemon

import (
	"context"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/logging"
	"github.com/RomanAgaltsev/gantry/internal/notify"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

// Observer records reconcile outcomes for metrics. C3a uses nopObserver; C3b provides a
// Prometheus implementation.
type Observer interface {
	ReconcileDone(env string, res engine.SyncResult, err error, dur time.Duration)
	DriftObserved(env string, ageSeconds float64)
}

type nopObserver struct{}

func (nopObserver) ReconcileDone(string, engine.SyncResult, error, time.Duration) {}
func (nopObserver) DriftObserved(string, float64)                                 {}

// Deps are the long-lived collaborators a reconcile needs, built once by the serve command.
type Deps struct {
	Cfg              *config.Config
	Forge            forge.Forge
	Store            engine.PinStore
	Ledger           ledger.Ledger
	Dispatch         notify.Dispatcher
	ExecFor          func(env config.Environment) (executor.Executor, verify.Verifier, error)
	Metrics          Observer      // nil ⇒ nopObserver
	ReconcileTimeout time.Duration // per-env reconcile deadline; 0 ⇒ no deadline
}

// Options tunes the loop.
type Options struct {
	Interval time.Duration
	Doorbell <-chan struct{} // C3c; nil ⇒ interval only
}

// Run drives the reconcile loop until ctx is cancelled, then returns nil. A reconcile error
// never returns from Run — it is logged, observed, and notified, and the loop continues.
func Run(ctx context.Context, d Deps, o Options) error {
	if d.Metrics == nil {
		d.Metrics = nopObserver{}
	}
	log := logging.From(ctx)
	log.Info("daemon started", "interval", o.Interval.String())
	ticker := time.NewTicker(o.Interval)
	defer ticker.Stop()

	reconcileAll(ctx, d) // reconcile once at startup, before the first tick
	for {
		select {
		case <-ctx.Done():
			log.Info("daemon stopping")
			return nil
		case <-ticker.C:
			reconcileAll(ctx, d)
		case <-o.Doorbell:
			log.Info("doorbell rung; reconciling")
			reconcileAll(ctx, d)
		}
	}
}

func reconcileAll(ctx context.Context, d Deps) {
	for _, env := range d.Cfg.Environments {
		if env.Source.Track == "" {
			continue // promote-mode envs are advanced by humans, never on a timer (C3-D8)
		}
		reconcileEnv(ctx, d, env)
	}
	observeDrift(ctx, d)
}

func reconcileEnv(ctx context.Context, d Deps, env config.Environment) {
	log := logging.From(ctx)
	ex, vf, err := d.ExecFor(env)
	if err != nil {
		log.Warn("skipping environment; executor build failed", "env", env.Name, "error", err)
		return
	}
	// Bound each environment's reconcile so a wedged remote command (e.g. a stuck
	// `docker compose pull`) cannot block the whole loop until shutdown (C2). The SSH
	// runner unblocks on ctx cancellation.
	if d.ReconcileTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.ReconcileTimeout)
		defer cancel()
	}
	start := time.Now()
	res, err := engine.Sync(ctx, d.Cfg, env.Name, d.Forge, ex, vf, d.Store, d.Ledger, engine.SyncOptions{})
	dur := time.Since(start)
	d.Metrics.ReconcileDone(env.Name, res, err, dur)
	if err != nil {
		log.Error("reconcile failed", "env", env.Name, "error", err)
	} else if res.Deployed {
		log.Info("reconciled", "env", env.Name, "changes", len(res.Changes))
	}
	d.Dispatch.Dispatch(ctx, eventsFor(env.Name, res, err)...)
}

func observeDrift(ctx context.Context, d Deps) {
	for _, env := range d.Cfg.Environments {
		if env.Source.Track == "" {
			continue
		}
		rep, err := engine.Drift(ctx, d.Cfg, env.Name, d.Forge, d.Store)
		if err != nil {
			continue // drift is a best-effort signal in the loop; a forge blip is logged elsewhere
		}
		// Write the gauge every pass so it resets to 0 when drift resolves (C1); report the
		// max age (the oldest drifted component), not whichever item was last in config order.
		d.Metrics.DriftObserved(env.Name, maxDriftAge(rep))
		if rep.Drifted() {
			d.Dispatch.Dispatch(ctx, driftEvent(env.Name, rep)...)
		}
	}
}

// maxDriftAge returns the age in seconds of the oldest drifted component in rep, or 0 when
// nothing drifted (which clears the drift gauge for the environment).
func maxDriftAge(rep engine.DriftReport) float64 {
	var max float64
	for _, it := range rep.Items {
		if s := it.Age.Seconds(); s > max {
			max = s
		}
	}
	return max
}
