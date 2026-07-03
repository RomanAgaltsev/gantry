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

// Acquire creates path exclusively and writes "{pid}\n{unixNano}". If a fresh lock already
// exists it returns an error naming the holder; a stale lock (dead PID or older than
// staleAfter) is removed and re-acquired.
func Acquire(path string) (*Lock, error) {
	if err := reclaimIfStale(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("a gantry daemon is reconciling this repo (%s); stop it or wait", path)
		}
		return nil, err
	}
	defer f.Close()
	fmt.Fprintf(f, "%d\n%d\n", os.Getpid(), time.Now().UnixNano())
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

func reclaimIfStale(path string) error {
	pid, start, ok := readLock(path)
	if !ok {
		return nil
	}
	if isStale(pid, start) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
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

func isStale(pid int, start time.Time) bool {
	if time.Since(start) > staleAfter {
		return true
	}
	return !processAlive(pid)
}
