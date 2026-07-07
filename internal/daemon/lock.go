// Package daemon runs gantry's reconcile loop (`gantry serve`): it calls the same engine
// verbs the CLI does, on an interval, under a single-writer lock.
package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// staleAfter treats a lock older than this as abandoned (backstop against PID reuse); far
// longer than any reconcile interval.
const staleAfter = 24 * time.Hour

// Lock is an advisory single-writer lock on a repo, backed by a lockfile holding the owner's
// PID and start time. A fresh lock blocks a second Acquire; a stale one is reclaimed.
type Lock struct{ path string }

// heldErr is the error returned when another live process owns the lock.
func heldErr(path string) error {
	return fmt.Errorf("a gantry daemon is reconciling this repo (%s); stop it or wait", path)
}

// Acquire creates path exclusively and writes "{pid}\n{unixNano}". If a fresh lock already
// exists it returns an error naming the holder; a stale lock (dead PID or older than
// staleAfter) is removed and re-acquired.
//
// O_EXCL create is the single-winner primitive, but a concurrent stale-reclaim can rename a
// just-created fresh lock out from under its owner. To stay race-free under concurrent
// reclaim, Acquire writes its owner line then re-reads path and confirms it still holds its
// own identity: if a reclaim racer stole our file, we lost the race and report held rather
// than handing back a Lock whose file is already gone (C6).
func Acquire(path string) (*Lock, error) {
	if err := reclaimIfStale(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, heldErr(path)
		}
		return nil, err
	}
	owner := fmt.Sprintf("%d\n%d\n", os.Getpid(), time.Now().UnixNano())
	if _, err := f.WriteString(owner); err != nil {
		_ = f.Close() //nolint:gosec // best-effort close before returning the write error
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	// Re-verify: a concurrent stale-reclaim may have renamed our just-written lock away and
	// restored another holder. If path no longer holds our identity we lost the race.
	if got, rerr := os.ReadFile(path); rerr != nil || string(got) != owner {
		return nil, heldErr(path)
	}
	return &Lock{path: path}, nil
}

// Release removes the lockfile. Idempotent.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	err := os.Remove(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CheckFree returns an error when a fresh serve lock exists at path (a live daemon owns the
// repo); nil when the path is free or the lock is stale. Used by CLI mutating verbs.
func CheckFree(path string) error {
	pid, start, ok := readLock(path)
	if !ok || isStale(pid, start) {
		return nil
	}
	return fmt.Errorf("a gantry daemon is reconciling this repo (%s); retry when it is stopped", path)
}

// reclaimIfStale steals a stale lock so Acquire's O_EXCL create can proceed. It renames the
// lockfile to a unique temp (only one concurrent renamer can win) and removes the renamed
// copy. The rename is atomic but unconditional, so a fresh lock created between the staleness
// read and the rename would be renamed away too — to avoid orphaning a live holder, the
// renamed copy is re-validated: if it turned out fresh, it is restored to path and the lock is
// left in place. O_EXCL in Acquire remains the single-winner primitive.
func reclaimIfStale(path string) error {
	pid, start, ok := readLock(path)
	if !ok || !isStale(pid, start) {
		return nil
	}
	tmp := fmt.Sprintf("%s.stale.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.Rename(path, tmp); err != nil {
		if os.IsNotExist(err) {
			return nil // another racer already stole it
		}
		return err
	}
	// Re-validate what we renamed: a fresh lock may have appeared at path between our read and
	// the rename. If so, restore it so its live owner keeps it, and leave reclaim to Release.
	if rpid, rstart, rok := readLock(tmp); rok && !isStale(rpid, rstart) {
		_ = os.Rename(tmp, path) //nolint:gosec // best-effort restore of the live holder's lock
		return nil
	}
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readLock(path string) (pid int, start time.Time, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, time.Time{}, false
	}
	lines := strings.SplitN(strings.TrimSpace(string(b)), "\n", 2)
	if len(lines) != 2 {
		return 0, time.Time{}, false
	}
	pid, err1 := strconv.Atoi(strings.TrimSpace(lines[0]))
	ns, err2 := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, time.Time{}, false
	}
	return pid, time.Unix(0, ns), true
}

// isStale reports whether a lock is abandoned: older than staleAfter (a backstop against PID
// reuse, which processAlive cannot fully detect on Windows) or owned by a dead PID.
func isStale(pid int, start time.Time) bool {
	if time.Since(start) > staleAfter {
		return true
	}
	return !processAlive(pid)
}
