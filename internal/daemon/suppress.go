package daemon

import (
	"sync"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/notify"
)

// failureSuppressor decides whether a reconcile failure is worth notifying, collapsing a
// flapping host's repeated failures to one alert per window and emitting a single recovery
// signal when a failing environment next succeeds (review D7).
type failureSuppressor struct {
	window time.Duration
	mu     sync.Mutex
	state  map[string]envFailure
}

type envFailure struct {
	failing    bool
	lastNotify time.Time
}

func newFailureSuppressor(window time.Duration) *failureSuppressor {
	return &failureSuppressor{window: window, state: map[string]envFailure{}}
}

// ShouldNotify records the latest outcome for env and returns whether to emit a
// reconcile_failed alert (emit) and whether this outcome is a recovery from a failing streak
// (recovered). A failure emits on the first occurrence and again once window has elapsed.
func (s *failureSuppressor) ShouldNotify(env string, failed bool, now time.Time) (emit, recovered bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state[env]
	switch {
	case failed:
		if !st.failing || now.Sub(st.lastNotify) >= s.window {
			st.failing, st.lastNotify = true, now
			s.state[env] = st
			return true, false
		}
		st.failing = true
		s.state[env] = st
		return false, false
	default: // success
		wasFailing := st.failing
		st.failing = false
		s.state[env] = st
		return false, wasFailing
	}
}

// filter drops a reconcile_failed event when suppressed and appends a recovery note when a
// failing environment recovers (D7). It is the seam between reconcileEnv and the suppressor.
func (s *failureSuppressor) filter(env string, evs []notify.Event, failed bool, now time.Time) []notify.Event {
	emit, recovered := s.ShouldNotify(env, failed, now)
	out := make([]notify.Event, 0, len(evs))
	for _, e := range evs {
		if e.Kind == "reconcile_failed" && !emit {
			continue
		}
		out = append(out, e)
	}
	if recovered {
		out = append(out, notify.Event{Kind: "deployed", Environment: env, Time: now, Message: "recovered: reconcile succeeded after prior failures"})
	}
	return out
}
