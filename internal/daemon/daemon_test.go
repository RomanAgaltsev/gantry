package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

func TestRun_ReconcilesTrackEnvsAndStopsOnCancel(t *testing.T) {
	spy := &spyExec{}
	d := Deps{
		Cfg:   twoEnvConfig(t), // one track env "test", one promote env "prod"
		Forge: fakeForge{rel: forge.Release{ImageRepository: "reg/svc", ImageTag: "v1"}},
		Store: newFakeStore(), Ledger: newFakeLedger(),
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
		Cfg: oneTrackEnv(t), Forge: errForge{}, Store: newFakeStore(), Ledger: newFakeLedger(),
		ExecFor: func(config.Environment) (executor.Executor, verify.Verifier, error) { return &spyExec{}, nil, nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	require.NoError(t, Run(ctx, d, Options{Interval: 5 * time.Millisecond})) // returns only on ctx deadline
}
