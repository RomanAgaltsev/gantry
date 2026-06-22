package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type gitStore struct {
	repoDir string
	repo    *git.Repository
	author  object.Signature
}

// NewGitStore opens repoDir as a git repo whose worktree holds the pin files.
func NewGitStore(repoDir string, author object.Signature) (PinStore, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return nil, fmt.Errorf("open git repo %q: %w", repoDir, err)
	}
	return &gitStore{repoDir: repoDir, repo: repo, author: author}, nil
}

func (s *gitStore) Read(pinFile string) (pin.Set, error) {
	b, err := os.ReadFile(filepath.Join(s.repoDir, pinFile))
	if os.IsNotExist(err) {
		return pin.Set{}, nil
	}
	if err != nil {
		return nil, err
	}
	return pin.Read(bytesReader(b))
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
	_, err = wt.Commit(msg, &git.CommitOptions{Author: &s.author})
	return err
}

func bytesReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
