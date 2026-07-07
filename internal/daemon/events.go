package daemon

import (
	"fmt"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/humanize"
	"github.com/RomanAgaltsev/gantry/internal/notify"
)

func eventsFor(env string, res engine.SyncResult, err error) []notify.Event {
	now := time.Now()
	switch {
	case err != nil && res.VerifyFailed && res.AutoRolledBack:
		return []notify.Event{
			{Kind: "verify_failed", Environment: env, Time: now, Message: "verify failed: " + err.Error()},
			{Kind: "rolled_back", Environment: env, Commit: res.RolledBackTo, Time: now, By: "auto-rollback"},
		}
	case err != nil && res.VerifyFailed:
		return []notify.Event{{Kind: "verify_failed", Environment: env, Time: now, Message: err.Error()}}
	case err != nil:
		return []notify.Event{{Kind: "reconcile_failed", Environment: env, Time: now, Message: "reconcile failed: " + err.Error()}}
	case res.Deployed:
		return []notify.Event{{Kind: "deployed", Environment: env, Time: now, Message: "reconciled"}}
	default:
		return nil // no change
	}
}

// driftEvent maps a drifted environment's report to one drift_alarm event per drifted
// component, mirroring the CLI mapping (C2). Dispatched best-effort by the loop.
func driftEvent(env string, rep engine.DriftReport) []notify.Event {
	evs := make([]notify.Event, 0, len(rep.Items))
	for _, it := range rep.Items {
		evs = append(evs, notify.Event{
			Kind: "drift_alarm", Environment: env, Time: time.Now(),
			Message: fmt.Sprintf("drift: %s in %s is %s behind latest (%s)",
				it.Component, env, humanize.Duration(it.Age), it.Latest.SemverVersion),
		})
	}
	return evs
}
