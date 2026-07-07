package ledger

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/RomanAgaltsev/gantry/internal/gitutil"
)

const ledgerRelPath = ".gantry/deploys.jsonl"

type gitLedger struct {
	repoDir string
	repo    *git.Repository
	author  object.Signature
}

// NewGitLedger opens repoDir as a git repo and records outcomes to .gantry/deploys.jsonl.
func NewGitLedger(repoDir string, author object.Signature) (Ledger, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return nil, fmt.Errorf("open git repo %q: %w", repoDir, err)
	}
	return &gitLedger{repoDir: repoDir, repo: repo, author: author}, nil
}

func (l *gitLedger) abs() string { return filepath.Join(l.repoDir, filepath.FromSlash(ledgerRelPath)) }

// Record appends one JSON line to the ledger file and commits it.
// ctx is accepted for seam symmetry; the local git impl is synchronous and does not observe it.
func (l *gitLedger) Record(_ context.Context, e Entry) error {
	wt, err := l.repo.Worktree()
	if err != nil {
		return err
	}
	if err := gitutil.AssertOwnsIndex(wt, ledgerRelPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.abs()), 0o750); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal ledger entry: %w", err)
	}
	f, err := os.OpenFile(l.abs(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append ledger: %w", errors.Join(err, f.Close()))
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close ledger: %w", err)
	}

	if _, err := wt.Add(ledgerRelPath); err != nil {
		return fmt.Errorf("git add ledger: %w", err)
	}
	author := l.author
	author.When = time.Now()
	msg := fmt.Sprintf("chore(ledger): %s %s@%.7s by %s", e.Result, e.Environment, e.PinCommit, e.By)
	if _, err := wt.Commit(msg, &git.CommitOptions{Author: &author}); err != nil {
		return fmt.Errorf("commit ledger: %w", err)
	}
	return nil
}

func (l *gitLedger) all() ([]Entry, error) {
	b, err := os.ReadFile(l.abs())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	var out []Entry
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse ledger line: %w", err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (l *gitLedger) Lookup(_ context.Context, env, sha string) (Entry, bool, error) {
	entries, err := l.all()
	if err != nil {
		return Entry{}, false, err
	}
	e, ok := lookup(entries, env, sha)
	return e, ok, nil
}

func (l *gitLedger) LatestGreen(_ context.Context, env string) (Entry, error) {
	entries, err := l.all()
	if err != nil {
		return Entry{}, err
	}
	e, ok := latestGreen(entries, env)
	if !ok {
		return Entry{}, ErrNoGreen
	}
	return e, nil
}

func (l *gitLedger) History(_ context.Context, env string) ([]Entry, error) {
	entries, err := l.all()
	if err != nil {
		return nil, err
	}
	return history(entries, env), nil
}

func (l *gitLedger) LatestHealthy(_ context.Context, env string) (Entry, error) {
	entries, err := l.all()
	if err != nil {
		return Entry{}, err
	}
	e, ok := latestHealthy(entries, env)
	if !ok {
		return Entry{}, ErrNoGreen
	}
	return e, nil
}
