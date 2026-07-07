//go:build !windows
// +build !windows

package daemon

import (
	"errors"
	"os"
	"syscall"
)

// lockExclNB takes an exclusive, non-blocking flock on f. It returns nil if the lock was
// acquired and errLockHeld if another process (or goroutine, which holds a distinct open file
// description) already holds it.
func lockExclNB(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errLockHeld
		}
		return err
	}
	return nil
}

// lockSharedNB takes a shared, non-blocking flock on f, used by CheckFree's probe: a shared
// lock coexists with other shared probes but conflicts with an exclusive holder, which is
// exactly the "is a daemon running?" question. errLockHeld signals a live exclusive holder.
func lockSharedNB(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errLockHeld
		}
		return err
	}
	return nil
}
