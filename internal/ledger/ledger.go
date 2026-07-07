// Package ledger records and queries the outcome of every deploy gantry performs.
// It is gantry's source of truth for "what was deployed to an environment and did it
// succeed" - the gate for promotion and the basis for rollback, drift, and status.
package ledger

import (
	"errors"
	"time"
)

// ErrNoGreen is returned by LatestGreen when an environment has no ok deploy record.
var ErrNoGreen = errors.New("no green deploy recorded")

// Result is a deploy outcome as recorded in the ledger. Declared as a string type so the
// compiler checks comparisons while the on-disk JSONL wire format stays the literal strings.
type Result string

const (
	ResultOK     Result = "ok"
	ResultFailed Result = "failed"
)

// Health is a post-deploy verification verdict. "unknown" means no verify ran (A2 behavior).
type Health string

const (
	HealthUnknown Health = "unknown"
	HealthTrue    Health = "true"
	HealthFalse   Health = "false"
)

// Entry is one deploy outcome, append-only, keyed by (Environment, PinCommit).
type Entry struct {
	Environment string            `json:"environment"`
	PinCommit   string            `json:"pin_commit"`
	Result      Result            `json:"result"`  // ResultOK | ResultFailed
	Healthy     Health            `json:"healthy"` // HealthUnknown (A2) | HealthTrue | HealthFalse (B3)
	ImageSet    map[string]string `json:"image_set"`
	DeployedAt  time.Time         `json:"deployed_at"`
	By          string            `json:"by"` // "sync" | "deploy" | "promote" | "rollback"
}

// Ledger records and queries deploy outcomes.
type Ledger interface {
	// Record appends one outcome and persists it (the git impl commits the ledger file).
	Record(e Entry) error
	// Lookup returns the latest entry for (env, sha); ok is false if none exists.
	Lookup(env, sha string) (Entry, bool, error)
	// LatestGreen returns the most recent Result=="ok" entry for env, or ErrNoGreen.
	LatestGreen(env string) (Entry, error)
	// History returns every entry for env, newest first.
	History(env string) ([]Entry, error)
	// LatestHealthy returns the most recent ok+healthy entry for env, or ErrNoGreen.
	LatestHealthy(env string) (Entry, error)
}

// lookup returns the most recent entry matching (env, sha). Append-only, latest-wins.
// It scans from the tail so a retried deploy's fresh entry supersedes an earlier one.
func lookup(entries []Entry, env, sha string) (Entry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Environment == env && entries[i].PinCommit == sha {
			return entries[i], true
		}
	}
	return Entry{}, false
}

// latestGreen returns the most recent ok entry for env.
func latestGreen(entries []Entry, env string) (Entry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Environment == env && entries[i].Result == ResultOK {
			return entries[i], true
		}
	}
	return Entry{}, false
}

// latestHealthy returns the most recent ok entry for env whose verification passed.
func latestHealthy(entries []Entry, env string) (Entry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Environment == env && entries[i].Result == ResultOK && entries[i].Healthy == HealthTrue {
			return entries[i], true
		}
	}
	return Entry{}, false
}

// history returns every entry for env, newest first.
func history(entries []Entry, env string) []Entry {
	var out []Entry
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Environment == env {
			out = append(out, entries[i])
		}
	}
	return out
}
