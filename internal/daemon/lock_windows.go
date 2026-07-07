//go:build windows
// +build windows

package daemon

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Windows has no POSIX flock; the equivalent is LockFileEx on a byte range of the file. Locks
// are keyed to the open file handle and its underlying file object, so (as with flock) two
// handles opened by separate processes — or two handles in the same process — contend for the
// same range, which is what makes the single-winner test meaningful in-process. Closing the
// handle releases the lock, so Release is just Close and a crashed process is reclaimed by the
// OS without any userspace staleness heuristic.
var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = kernel32.NewProc("LockFileEx")
)

const (
	lockfileFailImmediate = 0x00000001 // LOCKFILE_FAIL_IMMEDIATELY: fail at once rather than wait.
	lockfileExclusiveLock = 0x00000002 // LOCKFILE_EXCLUSIVE_LOCK: writers exclude all others.

	// errLockViolation is ERROR_LOCK_VIOLATION (33): LockFileEx reports it when another owner
	// holds a conflicting range and LOCKFILE_FAIL_IMMEDIATELY was requested.
	errLockViolation = syscall.Errno(33)
)

// lockFileEx requests a Windows byte-range lock over the first byte of f. flags selects
// exclusive-vs-shared and the fail-immediate behaviour. It returns nil on success, errLockHeld
// on a conflicting existing lock, or the OS error otherwise.
func lockFileEx(f *os.File, flags uint32) error {
	var ol syscall.Overlapped // Offset/OffsetHigh zero ⇒ byte 0; no event ⇒ never waits.
	r1, _, e1 := syscall.SyscallN(
		procLockFileEx.Addr(),
		f.Fd(),
		uintptr(flags),
		0, // reserved
		1, // nNumberOfBytesToLockLow: lock exactly one byte at offset 0.
		0, // nNumberOfBytesToLockHigh
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		if errors.Is(e1, errLockViolation) {
			return errLockHeld
		}
		return e1
	}
	return nil
}

// lockExclNB takes an exclusive, fail-fast lock on f (the single-writer lock for `serve`).
func lockExclNB(f *os.File) error {
	return lockFileEx(f, lockfileExclusiveLock|lockfileFailImmediate)
}

// lockSharedNB takes a shared, fail-fast lock on f for CheckFree's probe: it conflicts with an
// exclusive holder but coexists with other shared probes.
func lockSharedNB(f *os.File) error {
	return lockFileEx(f, lockfileFailImmediate)
}
