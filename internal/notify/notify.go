// Package notify delivers gantry's deploy/promote/rollback/verify/drift events to
// configured channels (webhook, email), best-effort.
package notify

import (
	"context"
	"sync"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/logging"
)

const defaultChannelTimeout = 10 * time.Second

// Event kinds gantry can emit. The config validator and the event constructors reference
// these so a new kind is added in exactly one place.
const (
	KindDeployed        = "deployed"
	KindPromoted        = "promoted"
	KindRolledBack      = "rolled_back"
	KindVerifyFailed    = "verify_failed"
	KindDriftAlarm      = "drift_alarm"
	KindReconcileFailed = "reconcile_failed"
)

// Event is one thing gantry did (or failed to do) worth reporting. Message is the rendered
// single line; the other fields populate structured payloads (e.g. the webhook JSON).
type Event struct {
	Kind        string // deployed | promoted | rolled_back | verify_failed | drift_alarm
	Environment string
	Commit      string
	By          string
	Message     string
	Time        time.Time
}

// Notifier delivers one event to one destination.
type Notifier interface {
	Notify(ctx context.Context, e Event) error
}

// Channel is a notifier plus the event kinds it wants. An empty (nil) Events set means all.
type Channel struct {
	Notifier Notifier
	Events   map[string]bool
}

func (c Channel) wants(kind string) bool {
	if len(c.Events) == 0 {
		return true
	}
	return c.Events[kind]
}

// Dispatcher fans events out to channels.
type Dispatcher []Channel

// Dispatch sends each event to every subscribed channel. For a single event the subscribed
// channels are sent concurrently (P4) so slow destinations don't serialize a one-shot CLI run;
// events themselves are processed in order. Each send is bounded by a per-channel timeout and
// best-effort: a channel error is logged via the context logger and skipped, never returned, so
// a broken notification destination can never fail a gantry command. Dispatch runs synchronously
// so a one-shot CLI run finishes sending before the process exits.
func (d Dispatcher) Dispatch(ctx context.Context, events ...Event) {
	log := logging.From(ctx)
	for _, e := range events {
		var wg sync.WaitGroup
		for _, ch := range d {
			if !ch.wants(e.Kind) {
				continue
			}
			wg.Add(1)
			go func(ch Channel) {
				defer wg.Done()
				cctx, cancel := context.WithTimeout(ctx, defaultChannelTimeout)
				defer cancel()
				if err := ch.Notifier.Notify(cctx, e); err != nil {
					log.Warn("notification failed", "event", e.Kind, "env", e.Environment, "error", err)
				}
			}(ch)
		}
		wg.Wait() // finish this event's fan-out before the next, and before the CLI process exits
	}
}
