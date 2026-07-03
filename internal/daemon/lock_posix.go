//go:build !windows
// +build !windows

package daemon

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether pid names a running process. It is the staleness signal for
// the single-writer lock: a lock is reclaimed only when this reports the holder gone, so it
// is deliberately conservative — any uncertainty is treated as "alive" so a live daemon's
// lock is never wrongly stolen (a false-stale is the only dangerous direction).
//
// A signal-0 probe delivers no signal; the kernel answers ESRCH precisely when the process
// no longer exists, and EPERM when it exists but is off-limits to the caller.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true // os.FindProcess never fails on POSIX; assume alive regardless.
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return false // no such process — clearly dead.
		}
		return true // EPERM (alive but off-limits) or anything unexpected — assume alive.
	}
	return true
}
