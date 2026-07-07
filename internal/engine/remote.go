package engine

import "context"

// RemoteSyncer is an optional PinStore capability: a store backed by a git remote can
// fast-forward-pull before a reconcile cycle and push after it, so multiple daemon clones of
// the same repo converge instead of splitting the ledger (review D1). The local-only store
// does not implement it.
type RemoteSyncer interface {
	PullFF(ctx context.Context) error
	Push(ctx context.Context) error
}
