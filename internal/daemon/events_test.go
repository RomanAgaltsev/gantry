package daemon

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/engine"
)

func TestEventsFor_PlainErrorIsReconcileFailed(t *testing.T) {
	evs := eventsFor("prod", engine.SyncResult{}, errors.New("ssh refused"))
	require.Len(t, evs, 1)
	require.Equal(t, "reconcile_failed", evs[0].Kind)
}
