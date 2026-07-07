package daemon

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// spyExec is a race-safe executor that records how many times it deployed and the last env.
// The loop runs in its own goroutine while the test reads these counters, so access is guarded.
type spyExec struct {
	mu          sync.Mutex
	n           int
	lastEnvName string
}

func (s *spyExec) Deploy(_ context.Context, p executor.Plan) (executor.Result, error) {
	s.mu.Lock()
	s.n++
	s.lastEnvName = p.Env
	s.mu.Unlock()
	return executor.Result{Changed: true}, nil
}

func (s *spyExec) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func (s *spyExec) lastEnv() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastEnvName
}

// fakeForge returns the same release for every component, regardless of which env is polling.
type fakeForge struct{ rel forge.Release }

func (f fakeForge) LatestRelease(_ context.Context, c forge.Component) (forge.Release, error) {
	r := f.rel
	r.Component = c.ID
	return r, nil
}

// errForge always fails to read a release, so a reconcile errors but the loop must continue.
type errForge struct{}

func (errForge) LatestRelease(context.Context, forge.Component) (forge.Release, error) {
	return forge.Release{}, errors.New("forge unavailable")
}

// fakeStore is a minimal in-memory engine.PinStore. Its current pins stay empty so every
// reconcile observes a diff and reaches the executor.
type fakeStore struct {
	cur       pin.Set
	committed pin.Set
	headSHA   string
}

func newFakeStore() *fakeStore { return &fakeStore{} }

func (s *fakeStore) Read(string) (pin.Set, error)           { return s.cur, nil }
func (s *fakeStore) ReadAt(string, string) (pin.Set, error) { return pin.Set{}, nil }
func (s *fakeStore) WriteAndCommit(_ string, set pin.Set, _ string) (string, error) {
	s.committed = set
	s.headSHA = "newsha"
	return "newsha", nil
}

func (s *fakeStore) LatestCommit(string) (string, error) {
	if s.headSHA == "" {
		return "", engine.ErrNoHistory
	}
	return s.headSHA, nil
}

func (s *fakeStore) ParentOf(string) (string, error)    { return "", engine.ErrNoParent }
func (s *fakeStore) Resolve(rev string) (string, error) { return rev, nil }

// fakeLedger is an in-memory ledger.Ledger (latest-wins lookups, append-only records).
type fakeLedger struct{ entries []ledger.Entry }

func newFakeLedger() *fakeLedger { return &fakeLedger{} }

func (l *fakeLedger) Record(e ledger.Entry) error {
	l.entries = append(l.entries, e)
	return nil
}

func (l *fakeLedger) Lookup(env, sha string) (ledger.Entry, bool, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].PinCommit == sha {
			return l.entries[i], true, nil
		}
	}
	return ledger.Entry{}, false, nil
}

func (l *fakeLedger) LatestGreen(env string) (ledger.Entry, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].Result == ledger.ResultOK {
			return l.entries[i], nil
		}
	}
	return ledger.Entry{}, ledger.ErrNoGreen
}

func (l *fakeLedger) History(env string) ([]ledger.Entry, error) {
	var out []ledger.Entry
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env {
			out = append(out, l.entries[i])
		}
	}
	return out, nil
}

func (l *fakeLedger) LatestHealthy(env string) (ledger.Entry, error) {
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].Environment == env && l.entries[i].Result == ledger.ResultOK && l.entries[i].Healthy == ledger.HealthTrue {
			return l.entries[i], nil
		}
	}
	return ledger.Entry{}, ledger.ErrNoGreen
}

// twoEnvConfig has one track env "test" and one promote env "prod"; only "test" reconciles.
func twoEnvConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Components: []config.Component{{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE"}},
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
			{Name: "prod", Source: config.Source{PromoteFrom: "test"}, PinFile: ".env.versions.prod"},
		},
	}
}

// oneTrackEnv has a single track env "test".
func oneTrackEnv(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Components: []config.Component{{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE"}},
		Environments: []config.Environment{
			{Name: "test", Source: config.Source{Track: "latest"}, PinFile: ".env.versions.test"},
		},
	}
}
