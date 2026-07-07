package daemon

import (
	"context"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/executor"
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
	Engine                *engine.Engine
	Dispatch              notify.Dispatcher
	ExecFor               func(ctx context.Context, env config.Environment) (executor.Executor, verify.Verifier, error)
	Metrics               Observer           // nil ⇒ nopObserver
	ReconcileTimeout      time.Duration      // per-env reconcile deadline; 0 ⇒ no deadline
	Suppressor            *failureSuppressor // nil ⇒ default built in Run from ReconcileFailedRepeat
	ReconcileFailedRepeat time.Duration      // window for collapsing flapping reconcile_failed alerts
	RemotePull            bool               // ff-only pull from git.remote before each cycle (D1)
	RemotePush            bool               // push to git.remote after a cycle that committed (D1)
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
	if d.Suppressor == nil {
		window := d.ReconcileFailedRepeat
		if window <= 0 {
			window = time.Hour
		}
		d.Suppressor = newFailureSuppressor(window)
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
	syncer, canSync := d.Engine.Store.(engine.RemoteSyncer)
	if d.RemotePull && canSync {
		if err := syncer.PullFF(ctx); err != nil {
			// A divergence or transport error must not merge or crash: skip this cycle's work
			// and alert, so the operator sees the split instead of silent drift (D1).
			logging.From(ctx).Error("remote pull failed; skipping reconcile cycle", "error", err)
			d.Dispatch.Dispatch(ctx, notify.Event{Kind: notify.KindReconcileFailed, Environment: "*", Time: time.Now(), Message: "remote pull failed: " + err.Error()})
			return
		}
	}
	committed := false
	for _, env := range d.Engine.Cfg.Environments {
		if env.Source.Track == "" {
			continue // promote-mode envs are advanced by humans, never on a timer (C3-D8)
		}
		if reconcileEnv(ctx, d, env) {
			committed = true
		}
	}
	observeDrift(ctx, d)
	if d.RemotePush && canSync && committed {
		if err := syncer.Push(ctx); err != nil {
			logging.From(ctx).Error("remote push failed", "error", err)
			d.Dispatch.Dispatch(ctx, notify.Event{Kind: notify.KindReconcileFailed, Environment: "*", Time: time.Now(), Message: "remote push failed: " + err.Error()})
		}
	}
}

func reconcileEnv(ctx context.Context, d Deps, env config.Environment) bool {
	log := logging.From(ctx)
	ex, vf, err := d.ExecFor(ctx, env)
	if err != nil {
		log.Warn("skipping environment; executor build failed", "env", env.Name, "error", err)
		return false
	}
	defer func() {
		// The runner caches a pooled SSH connection; the daemon builds a fresh executor per env
		// per cycle, so releasing it here avoids leaking one TCP+SSH connection per deploying
		// cycle (C3). The one-shot CLI still relies on process exit.
		if rc, ok := ex.(executor.RunnerCloser); ok {
			if cerr := rc.CloseRunner(); cerr != nil {
				logging.From(ctx).Warn("closing runner after reconcile", "env", env.Name, "error", cerr)
			}
		}
	}()
	// Bound each environment's reconcile so a wedged remote command (e.g. a stuck
	// `docker compose pull`) cannot block the whole loop until shutdown (C2). The SSH
	// runner unblocks on ctx cancellation.
	if d.ReconcileTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.ReconcileTimeout)
		defer cancel()
	}
	start := time.Now()
	res, err := d.Engine.Sync(ctx, env.Name, ex, vf, engine.SyncOptions{})
	dur := time.Since(start)
	d.Metrics.ReconcileDone(env.Name, res, err, dur)
	if err != nil {
		log.Error("reconcile failed", "env", env.Name, "error", err)
	} else if res.Deployed {
		log.Info("reconciled", "env", env.Name, "changes", len(res.Changes))
	}
	evs := eventsFor(env.Name, res, err)
	// Collapse flapping reconcile_failed alerts to one per window; emit a recovery note on
	// the first success after a failing streak (D7).
	if d.Suppressor != nil {
		evs = d.Suppressor.filter(env.Name, evs, err != nil, time.Now())
	}
	d.Dispatch.Dispatch(ctx, evs...)
	return res.Deployed
}

func observeDrift(ctx context.Context, d Deps) {
	for _, env := range d.Engine.Cfg.Environments {
		if env.Source.Track == "" {
			continue
		}
		rep, err := d.Engine.Drift(ctx, env.Name)
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
