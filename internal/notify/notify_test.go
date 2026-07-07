package notify

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
)

type recordingNotifier struct {
	got []Event
	err error
}

func (n *recordingNotifier) Notify(_ context.Context, e Event) error {
	n.got = append(n.got, e)
	return n.err
}

func TestDispatch_SubscriptionFilter(t *testing.T) {
	all := &recordingNotifier{}
	failsOnly := &recordingNotifier{}
	d := Dispatcher{
		{Notifier: all}, // nil Events = all kinds
		{Notifier: failsOnly, Events: map[string]bool{"verify_failed": true}},
	}
	d.Dispatch(
		context.Background(),
		Event{Kind: "deployed", Environment: "test"},
		Event{Kind: "verify_failed", Environment: "test"},
	)
	require.Len(t, all.got, 2)       // subscribed to everything
	require.Len(t, failsOnly.got, 1) // only verify_failed
	require.Equal(t, "verify_failed", failsOnly.got[0].Kind)
}

func TestDispatch_BestEffort(t *testing.T) {
	boom := &recordingNotifier{err: errors.New("network down")}
	ok := &recordingNotifier{}
	d := Dispatcher{{Notifier: boom}, {Notifier: ok}}
	// Must not panic or stop at the failing channel, and returns nothing.
	d.Dispatch(context.Background(), Event{Kind: "deployed"})
	require.Len(t, ok.got, 1) // the second channel still received it
}

type notifierFunc func(context.Context, Event) error

func (f notifierFunc) Notify(ctx context.Context, e Event) error { return f(ctx, e) }

func waitTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("timed out waiting for concurrent notifiers")
	}
}

func TestDispatch_SendsChannelsConcurrently(t *testing.T) {
	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	barrier := make(chan struct{})
	var got int64
	mk := func() Notifier {
		return notifierFunc(func(context.Context, Event) error {
			wg.Done()
			<-barrier // block until all n have arrived ⇒ proves concurrency
			atomic.AddInt64(&got, 1)
			return nil
		})
	}
	d := make(Dispatcher, 0, n)
	for range n {
		d = append(d, Channel{Notifier: mk()})
	}
	done := make(chan struct{})
	go func() { d.Dispatch(context.Background(), Event{Kind: "deployed"}); close(done) }()

	// All n notifiers must be in-flight simultaneously (sequential dispatch would deadlock here).
	waitTimeout(t, &wg, time.Second)
	close(barrier)
	<-done
	require.Equal(t, int64(n), atomic.LoadInt64(&got))
}

// TestEventKindsMatchConfigValidation guards the single-source-of-truth contract between the
// notify.Kind* string constants and config.NotifyEventKinds (review §2.2-B): a kind emitted by
// any event constructor must be accepted by config validation, and vice versa.
func TestEventKindsMatchConfigValidation(t *testing.T) {
	for _, k := range []string{
		KindDeployed, KindPromoted, KindRolledBack,
		KindVerifyFailed, KindDriftAlarm, KindReconcileFailed,
	} {
		require.True(t, config.NotifyEventKinds[k], "config validation must accept kind %q", k)
	}
	require.Len(t, config.NotifyEventKinds, 6)
}
