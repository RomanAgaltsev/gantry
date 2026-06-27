package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/RomanAgaltsev/gantry/internal/gitutil"
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

// Read returns the pin set as committed at HEAD. A repo with no commits reads empty.
func (s *gitStore) Read(pinFile string) (pin.Set, error) {
	ref, err := s.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return pin.Set{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	return s.ReadAt(ref.Hash().String(), pinFile)
}

// ReadAt returns the pin set as committed at sha. A commit that does not track the
// file reads as an empty set.
func (s *gitStore) ReadAt(sha, pinFile string) (pin.Set, error) {
	commit, err := s.repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return nil, fmt.Errorf("load commit %s: %w", sha, err)
	}
	f, err := commit.File(filepath.ToSlash(pinFile))
	if errors.Is(err, object.ErrFileNotFound) {
		return pin.Set{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %q at %s: %w", pinFile, sha, err)
	}
	contents, err := f.Contents()
	if err != nil {
		return nil, fmt.Errorf("read %q at %s: %w", pinFile, sha, err)
	}
	return pin.Read(bytes.NewReader([]byte(contents)))
}

// LatestCommit returns the SHA of the most recent commit that touched pinFile.
func (s *gitStore) LatestCommit(pinFile string) (string, error) {
	name := filepath.ToSlash(pinFile)
	iter, err := s.repo.Log(&git.LogOptions{FileName: &name})
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return "", ErrNoHistory
	}
	if err != nil {
		return "", fmt.Errorf("log %q: %w", pinFile, err)
	}
	defer iter.Close()
	c, err := iter.Next()
	if errors.Is(err, io.EOF) {
		return "", ErrNoHistory
	}
	if err != nil {
		return "", fmt.Errorf("log %q: %w", pinFile, err)
	}
	return c.Hash.String(), nil
}

// ParentOf returns the first-parent SHA of sha, or ErrNoParent for a root commit.
func (s *gitStore) ParentOf(sha string) (string, error) {
	commit, err := s.repo.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return "", fmt.Errorf("load commit %s: %w", sha, err)
	}
	if len(commit.ParentHashes) == 0 {
		return "", ErrNoParent
	}
	return commit.ParentHashes[0].String(), nil
}

// WriteAndCommit writes pinFile, stages it, and commits, returning the new commit SHA.
func (s *gitStore) WriteAndCommit(pinFile string, set pin.Set, msg string) (string, error) {
	wt, err := s.repo.Worktree()
	if err != nil {
		return "", err
	}
	if err := gitutil.AssertOwnsIndex(wt, pinFile); err != nil {
		return "", err
	}
	abs := filepath.Join(s.repoDir, pinFile)
	if err := os.WriteFile(abs, pin.Render(set), 0o600); err != nil {
		return "", err
	}
	if _, err := wt.Add(pinFile); err != nil {
		return "", err
	}
	author := s.author
	author.When = time.Now()
	hash, err := wt.Commit(msg, &git.CommitOptions{Author: &author})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}
