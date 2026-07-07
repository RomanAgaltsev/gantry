package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestAcquire_StaleLockStolenByExactlyOneRacer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.lock")
	// Plant a stale lock: an almost-certainly-dead PID and a timestamp older than staleAfter.
	stale := fmt.Sprintf("%d\n%d\n", 999999999, time.Now().Add(-48*time.Hour).UnixNano())
	require.NoError(t, os.WriteFile(path, []byte(stale), 0o600))

	const racers = 8
	var (
		mu    sync.Mutex
		locks []*Lock
		wg    sync.WaitGroup
	)
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l, err := Acquire(path); err == nil {
				mu.Lock()
				locks = append(locks, l)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Len(t, locks, 1, "exactly one racer may win a stale lock")
	for _, l := range locks {
		_ = l.Release()
	}
}
