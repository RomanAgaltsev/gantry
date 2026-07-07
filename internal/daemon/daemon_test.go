package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

func TestRun_ReconcilesTrackEnvsAndStopsOnCancel(t *testing.T) {
	spy := &spyExec{}
	d := Deps{
		Engine:  engine.New(twoEnvConfig(t), fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}, newFakeStore(), newFakeLedger()),
		ExecFor: func(config.Environment) (executor.Executor, verify.Verifier, error) { return spy, nil, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, d, Options{Interval: 5 * time.Millisecond}) }()

	require.Eventually(t, func() bool { return spy.calls() > 0 }, time.Second, 2*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err) // graceful stop
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
	require.Equal(t, "test", spy.lastEnv()) // only the track-mode env, never "prod" (C3-D8)
}

func TestRun_SurvivesReconcileError(t *testing.T) {
	// A forge that always errors must not stop the loop: it keeps ticking until cancel.
	d := Deps{
		Engine:  engine.New(oneTrackEnv(t), errForge{}, newFakeStore(), newFakeLedger()),
		ExecFor: func(config.Environment) (executor.Executor, verify.Verifier, error) { return &spyExec{}, nil, nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	require.NoError(t, Run(ctx, d, Options{Interval: 5 * time.Millisecond})) // returns only on ctx deadline
}

// recObserver records every ReconcileDone/DriftObserved call for assertions.
type recObserver struct {
	mu    sync.Mutex
	drift []driftCall
}

type driftCall struct {
	env string
	age float64
}

func (o *recObserver) ReconcileDone(string, engine.SyncResult, error, time.Duration) {}
func (o *recObserver) DriftObserved(env string, ageSeconds float64) {
	o.mu.Lock()
	o.drift = append(o.drift, driftCall{env, ageSeconds})
	o.mu.Unlock()
}

func TestObserveDrift_ReportsMaxAndClearsWhenResolved(t *testing.T) {
	// BuiltAt must be older than the 7-day default drift threshold to register as drift.
	old := forge.Release{ImageRepository: "reg/svc", ImageTag: "v2", BuiltAt: time.Now().Add(-10 * 24 * time.Hour)}
	obs := &recObserver{}
	store := newFakeStore() // Read returns empty pins ⇒ latest ref differs ⇒ drift
	d := Deps{
		Engine:  engine.New(oneTrackEnv(t), fakeForge{rel: old}, store, newFakeLedger()),
		Metrics: obs,
	}
	// Pass 1: component is behind by 10d ⇒ one DriftObserved(env,>0).
	observeDrift(context.Background(), d)

	// Pass 2: pin is now at latest ⇒ nothing drifts ⇒ DriftObserved(env,0) must still fire.
	store.cur = pin.Set{"SVC_IMAGE": old.ImageRef()}
	observeDrift(context.Background(), d)

	obs.mu.Lock()
	defer obs.mu.Unlock()
	require.Len(t, obs.drift, 2, "gauge must be written every pass, even with no drift")
	require.Greater(t, obs.drift[0].age, 0.0)
	require.Equal(t, "test", obs.drift[0].env)
	require.Equal(t, 0.0, obs.drift[1].age, "gauge must reset to 0 when drift resolves")
}

// blockingExec blocks in Deploy until ctx is cancelled, modelling a wedged `compose pull`.
type blockingExec struct{}

func (blockingExec) Deploy(ctx context.Context, _ executor.Plan) (executor.Result, error) {
	<-ctx.Done()
	return executor.Result{}, ctx.Err()
}

func TestReconcileEnv_PerCycleTimeoutUnblocksWedgedDeploy(t *testing.T) {
	d := Deps{
		Engine:           engine.New(oneTrackEnv(t), fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}}, newFakeStore(), newFakeLedger()),
		ExecFor:          func(config.Environment) (executor.Executor, verify.Verifier, error) { return blockingExec{}, nil, nil },
		ReconcileTimeout: 20 * time.Millisecond,
	} // Dispatch left nil: Dispatcher.Dispatch on a nil slice is a no-op
	d.Metrics = nopObserver{} // reconcileEnv is driven directly, not via Run which sets the nop default

	done := make(chan struct{})
	go func() { reconcileEnv(context.Background(), d, d.Engine.Cfg.Environments[0]); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconcileEnv did not return; per-cycle timeout not applied")
	}
}
