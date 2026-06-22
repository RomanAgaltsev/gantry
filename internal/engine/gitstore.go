package engine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type gitStore struct {
	repoDir string
	repo    *git.Repository
	author  object.Signature
}

// NewGitStore opens repoDir as a git repo whose worktree holds the pin files.
// author supplies the commit identity; its timestamp is set per-commit.
func NewGitStore(repoDir string, author object.Signature) (PinStore, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return nil, fmt.Errorf("open git repo %q: %w", repoDir, err)
	}
	return &gitStore{repoDir: repoDir, repo: repo, author: author}, nil
}

// Read returns the pin set as committed at HEAD (not the working tree), matching
// gantry's GitOps contract that it operates on committed state. A repo with no
// commits, or one whose HEAD does not yet track pinFile, reads as an empty set.
func (s *gitStore) Read(pinFile string) (pin.Set, error) {
	ref, err := s.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return pin.Set{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	commit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("load HEAD commit: %w", err)
	}
	f, err := commit.File(filepath.ToSlash(pinFile))
	if errors.Is(err, object.ErrFileNotFound) {
		return pin.Set{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %q at HEAD: %w", pinFile, err)
	}
	contents, err := f.Contents()
	if err != nil {
		return nil, fmt.Errorf("read %q at HEAD: %w", pinFile, err)
	}
	return pin.Read(bytes.NewReader([]byte(contents)))
}

func (s *gitStore) WriteAndCommit(pinFile string, set pin.Set, msg string) error {
	abs := filepath.Join(s.repoDir, pinFile)
	if err := os.WriteFile(abs, pin.Render(set), 0o644); err != nil {
		return err
	}
	wt, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := wt.Add(pinFile); err != nil {
		return err
	}
	// Stamp the time at commit (not at construction) so a long-lived process
	// dates each commit correctly rather than reusing a stale time.
	author := s.author
	author.When = time.Now()
	_, err = wt.Commit(msg, &git.CommitOptions{Author: &author})
	return err
}
