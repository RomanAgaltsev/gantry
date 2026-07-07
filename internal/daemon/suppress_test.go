package daemon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFailureSuppressor(t *testing.T) {
	s := newFailureSuppressor(time.Hour)
	t0 := time.Now()

	emit, rec := s.ShouldNotify("prod", true, t0)
	require.True(t, emit) // first failure notifies
	require.False(t, rec)

	emit, _ = s.ShouldNotify("prod", true, t0.Add(10*time.Minute))
	require.False(t, emit) // within window ⇒ suppressed

	emit, _ = s.ShouldNotify("prod", true, t0.Add(2*time.Hour))
	require.True(t, emit) // window elapsed ⇒ re-notify

	emit, rec = s.ShouldNotify("prod", false, t0.Add(3*time.Hour))
	require.False(t, emit)
	require.True(t, rec) // recovery after a failing streak

	_, rec = s.ShouldNotify("prod", false, t0.Add(4*time.Hour))
	require.False(t, rec) // steady green ⇒ no repeated recovery
}
