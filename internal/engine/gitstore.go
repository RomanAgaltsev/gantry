package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/RomanAgaltsev/gantry/internal/gitutil"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type gitStore struct {
	repoDir      string
	repo         *git.Repository
	author       object.Signature
	remoteName   string
	remoteBranch string
	auth         transport.AuthMethod
}

// NewGitStore opens repoDir as a git repo whose worktree holds the pin files.
// author supplies the commit identity; its timestamp is set per-commit.
func NewGitStore(repoDir string, author object.Signature) (PinStore, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return nil, fmt.Errorf("open git repo %q: %w", repoDir, err)
	}
	return &gitStore{repoDir: repoDir, repo: repo, author: author, remoteName: "origin"}, nil
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

// Resolve expands a revision (short SHA, full SHA, branch, or tag) to a full commit SHA.
// It lets callers accept the abbreviated SHAs operators copy from `git log` / `gantry
// history` instead of demanding the full 40-character form.
func (s *gitStore) Resolve(rev string) (string, error) {
	h, err := s.repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return "", fmt.Errorf("resolve revision %q: %w", rev, err)
	}
	return h.String(), nil
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

// SetRemoteAuth configures the remote the store pulls from / pushes to and the transport auth
// used for it. username defaults to "gantry" when empty (a token-only HTTPS remote needs a
// non-empty username). The branch is informational (PullFF/Push use the current branch when
// empty). SSH/anonymous remotes leave token empty so no basic-auth method is set.
func (s *gitStore) SetRemoteAuth(username, token, remote, branch string) {
	s.remoteName = orDefault(remote, "origin")
	s.remoteBranch = branch
	if token != "" {
		s.auth = &githttp.BasicAuth{Username: orDefault(username, "gantry"), Password: token}
	}
}

// PullFF fast-forwards the worktree from the configured remote. A divergence (non-fast-forward)
// returns ErrNonFastForward rather than merging; an already-up-to-date pull is a no-op.
func (s *gitStore) PullFF(ctx context.Context) error {
	wt, err := s.repo.Worktree()
	if err != nil {
		return err
	}
	opts := &git.PullOptions{RemoteName: s.remoteName, Auth: s.auth, Force: false}
	if s.remoteBranch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(s.remoteBranch)
	}
	err = wt.PullContext(ctx, opts)
	switch {
	case err == nil, errors.Is(err, git.NoErrAlreadyUpToDate):
		return nil
	case errors.Is(err, git.ErrNonFastForwardUpdate):
		return ErrNonFastForward
	default:
		return fmt.Errorf("pull %q: %w", s.remoteName, err)
	}
}

// Push pushes the current branch to the configured remote. An up-to-date push is a no-op.
func (s *gitStore) Push(ctx context.Context) error {
	err := s.repo.PushContext(ctx, &git.PushOptions{RemoteName: s.remoteName, Auth: s.auth})
	if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return fmt.Errorf("push %q: %w", s.remoteName, err)
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
