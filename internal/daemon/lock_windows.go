//go:build windows
// +build windows

package daemon

import (
	"errors"
	"syscall"
)

// errInvalidParameter is ERROR_INVALID_PARAMETER (winerror 87): OpenProcess returns it for a
// pid that maps to no process (including 0). It is the one unambiguous "clearly dead" signal
// we act on; anything else — e.g. ERROR_ACCESS_DENIED for a live but higher-privilege
// process — is treated as alive so a live daemon's lock is never wrongly stolen.
const errInvalidParameter = syscall.Errno(87)

// processQueryLimitedInfo is PROCESS_QUERY_LIMITED_INFORMATION (0x1000): a right
// non-elevated callers can usually obtain, sufficient to test a process's existence.
const processQueryLimitedInfo = 0x1000

// processAlive reports whether pid names a running process. It is the staleness signal for
// the single-writer lock: a lock is reclaimed only when this reports the holder gone, so it
// is deliberately conservative. Windows has no signal-0 probe; this leans on OpenProcess and
// only treats ERROR_INVALID_PARAMETER (no such process) as "clearly dead" — every other
// outcome (including access-denied for a live, elevated process) is reported as alive.
//
// Windows pids are uint32; a value outside that range cannot name a process, so it is
// reported dead (the cast below is therefore overflow-safe).
const maxUint32 = 1<<32 - 1

func processAlive(pid int) bool {
	if pid < 0 || pid > maxUint32 {
		return false // not a valid Windows pid — cannot name a process.
	}
	h, err := syscall.OpenProcess(processQueryLimitedInfo, false, uint32(pid))
	if err == nil {
		_ = syscall.CloseHandle(h) //nolint:gosec // best-effort close; a failure cannot change the liveness answer
		return true
	}
	return !errors.Is(err, errInvalidParameter)
}
