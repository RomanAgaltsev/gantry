// Package gitutil holds small go-git helpers shared by gantry's git-backed stores.
package gitutil

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
)

// AssertOwnsIndex refuses to proceed when the repository has staged changes to any file
// other than allowPath. gantry builds each commit from the whole staging index (go-git
// commits the index, not a single path), so a pre-staged change would be silently folded
// into gantry's commit and corrupt the deploy history the tool exists to produce.
//
// Untracked files and unstaged worktree edits are ignored: neither is part of the index,
// so neither lands in gantry's commit. Only an already-staged change to another path is a
// problem, and that is what this rejects.
func AssertOwnsIndex(wt *git.Worktree, allowPath string) error {
	st, err := wt.Status()
	if err != nil {
		return fmt.Errorf("read worktree status: %w", err)
	}
	allow := filepath.ToSlash(allowPath)
	for path, fs := range st {
		if filepath.ToSlash(path) == allow {
			continue
		}
		if fs.Staging != git.Unmodified && fs.Staging != git.Untracked {
			return fmt.Errorf("refusing to commit: %q is already staged; gantry must own "+
				"the working tree (commit or unstage it first)", path)
		}
	}
	return nil
}
