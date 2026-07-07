// Package daemon runs gantry's reconcile loop (`gantry serve`): it calls the same engine
// verbs the CLI does, on an interval, under a single-writer lock.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Lock is an advisory single-writer lock on a repo, backed by an exclusive flock on a
// lockfile. The lock is held for as long as the owning process keeps the underlying file
// descriptor open (carried in Lock); closing it — explicitly via Release or implicitly when the
// process exits or crashes — drops the lock immediately, so a dead daemon's lock is reclaimed
// by the kernel without any staleness heuristic and never needs a userspace reclaim step. The
// file also records the owner's PID and start time for operator inspection.
type Lock struct{ f *os.File }

// errLockHeld is the platform-normalised "another process holds the lock" signal returned by
// the locking primitives; Acquire/CheckFree translate it into a human-readable heldErr.
var errLockHeld = errors.New("lock held")

// heldErr is the error returned when another live process owns the lock.
func heldErr(path string) error {
	return fmt.Errorf("a gantry daemon is reconciling this repo (%s); stop it or wait", path)
}

// ownerLine is the diagnostic content written into a held lockfile (PID + start time) so an
// operator can identify the holder. It is for inspection only — the locking logic never reads
// it; the flock is the lock.
func ownerLine() string {
	return fmt.Sprintf("%d\n%d\n", os.Getpid(), time.Now().UnixNano())
}

// Acquire takes the single-writer lock at path: it creates/opens the lockfile and takes an
// exclusive, non-blocking flock on it. The flock is the single-winner primitive — the kernel
// serialises all takers across processes and goroutines, so exactly one Acquire wins and every
// concurrent taker fails with heldErr. There is no stale-reclaim race: a crashed holder's lock
// is released by the kernel the instant it exits, so reclaim is automatic and unconditional
// rather than a racy read-then-steal.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockExclNB(f); err != nil {
		_ = f.Close() //nolint:gosec // best-effort close before reporting the lock as held
		if errors.Is(err, errLockHeld) {
			return nil, heldErr(path)
		}
		return nil, err
	}
	// Held: record who we are for operators. Best-effort — the lock is the flock, not this
	// content, so a write failure cannot undermine it.
	if _, err := f.Seek(0, 0); err == nil {
		_ = f.Truncate(0)                 //nolint:gosec // best-effort: the flock — not this content — is the lock
		_, _ = f.WriteString(ownerLine()) //nolint:gosec // best-effort: the flock — not this content — is the lock
	}
	return &Lock{f: f}, nil
}

// Release drops the lock by closing the file descriptor (which releases the flock) and leaves
// the lockfile on disk so its inode — and thus any flock on it — stays stable. Idempotent.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// CheckFree returns an error when a live daemon holds the lock (an exclusive flock is held on
// the lockfile); nil when the path is free. It takes a brief shared, non-blocking probe lock so
// that two concurrent CLI checks do not block one another, while an exclusive holder still
// reports as held. Used by CLI mutating verbs. Absent file ⇒ free.
func CheckFree(path string) error {
	f, err := os.Open(path) // no create: an absent file is definitionally unlocked
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close() // advisory check; closing also releases the probe lock
	if err := lockSharedNB(f); err != nil {
		if errors.Is(err, errLockHeld) {
			return fmt.Errorf("a gantry daemon is reconciling this repo (%s); retry when it is stopped", path)
		}
		return err
	}
	return nil
}
