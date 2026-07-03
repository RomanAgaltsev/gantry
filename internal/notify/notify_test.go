package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
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
