package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLock_AcquireBlocksSecond(t *testing.T) {
	p := filepath.Join(t.TempDir(), "serve.lock")
	l, err := Acquire(p)
	require.NoError(t, err)
	_, err = Acquire(p)
	require.ErrorContains(t, err, "reconciling") // names the holder
	require.NoError(t, l.Release())
	l2, err := Acquire(p) // free after release
	require.NoError(t, err)
	require.NoError(t, l2.Release())
}

func TestLock_StaleReclaimed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "serve.lock")
	// A lock for a PID that cannot exist is stale and reclaimable.
	require.NoError(t, os.WriteFile(p, []byte("999999999\n1\n"), 0o600))
	l, err := Acquire(p)
	require.NoError(t, err)
	require.NoError(t, l.Release())
}

func TestCheckFree(t *testing.T) {
	p := filepath.Join(t.TempDir(), "serve.lock")
	require.NoError(t, CheckFree(p)) // no lock
	l, err := Acquire(p)
	require.NoError(t, err)
	require.ErrorContains(t, CheckFree(p), "reconciling") // fresh lock
	require.NoError(t, l.Release())
	require.NoError(t, CheckFree(p)) // released
}
