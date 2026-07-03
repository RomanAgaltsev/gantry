package cli

import (
	"fmt"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/notify"
)

// verifyFailureEvents renders the events for a verb that failed its post-deploy verify. A
// plain deploy failure (verifyFailed false) yields nothing — there is no generic failure event.
func verifyFailureEvents(env string, verifyFailed, autoRolledBack bool, rolledBackTo string) []notify.Event {
	var evs []notify.Event
	if verifyFailed {
		evs = append(evs, notify.Event{
			Kind: "verify_failed", Environment: env, Time: time.Now(),
			Message: "verify failed for " + env,
		})
	}
	if autoRolledBack {
		evs = append(evs, notify.Event{
			Kind: "rolled_back", Environment: env, Commit: rolledBackTo, By: "auto-rollback", Time: time.Now(),
			Message: fmt.Sprintf("rolled back %s to %.7s", env, rolledBackTo),
		})
	}
	return evs
}

func deployEvents(env string, res engine.DeployResult, err error) []notify.Event {
	if err != nil {
		return verifyFailureEvents(env, res.VerifyFailed, res.AutoRolledBack, res.RolledBackTo)
	}
	if !res.Deployed {
		return nil
	}
	return []notify.Event{{
		Kind: "deployed", Environment: env, By: "deploy", Time: time.Now(),
		Message: fmt.Sprintf("deployed %d pin(s) to %s", len(res.Pins), env),
	}}
}

func syncEvents(env string, res engine.SyncResult, err error) []notify.Event {
	if err != nil {
		return verifyFailureEvents(env, res.VerifyFailed, res.AutoRolledBack, res.RolledBackTo)
	}
	if !res.Deployed {
		return nil
	}
	msg := fmt.Sprintf("deployed %d pin(s) to %s", len(res.Changes), env)
	if res.Recovered {
		msg = "redeployed the last committed pin set to " + env
	}
	return []notify.Event{{Kind: "deployed", Environment: env, By: "sync", Time: time.Now(), Message: msg}}
}

func promoteEvents(from, to string, res engine.PromoteResult, err error) []notify.Event {
	if err != nil {
		return verifyFailureEvents(to, res.VerifyFailed, res.AutoRolledBack, res.RolledBackTo)
	}
	if !res.Deployed {
		return nil
	}
	return []notify.Event{{
		Kind: "promoted", Environment: to, Commit: res.Committed, By: "promote", Time: time.Now(),
		Message: fmt.Sprintf("promoted %s@%.7s -> %s (%d pins)", from, res.FromSHA, to, len(res.Pins)),
	}}
}

func rollbackEvents(env string, res engine.RollbackResult, err error) []notify.Event {
	if err != nil || !res.Deployed {
		return nil
	}
	msg := fmt.Sprintf("rolled back %s to %.7s", env, res.ToSHA)
	if res.Slot != "" {
		msg = fmt.Sprintf("rolled back %s by switching to %s", env, res.Slot)
	}
	return []notify.Event{{Kind: "rolled_back", Environment: env, Commit: res.ToSHA, By: "rollback", Time: time.Now(), Message: msg}}
}

func driftEvents(rep engine.DriftReport) []notify.Event {
	evs := make([]notify.Event, 0, len(rep.Items))
	for _, it := range rep.Items {
		evs = append(evs, notify.Event{
			Kind: "drift_alarm", Environment: it.Env, Time: time.Now(),
			Message: fmt.Sprintf("drift: %s in %s is %s behind latest (%s)",
				it.Component, it.Env, humanizeDuration(it.Age), it.Latest.SemverVersion),
		})
	}
	return evs
}
