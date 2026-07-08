// Package executor defines the deploy backend interface and its plan/result types.
package executor

import (
	"context"

	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// Plan is a request to reconcile an environment to a pin set.
type Plan struct {
	Env     string
	PinFile string
	Pins    pin.Set
	Commit  string // pin-file commit SHA this plan deploys (used to name release dirs)
}

// Result reports the outcome of a deploy.
type Result struct {
	Changed bool
	Detail  string
}

// Executor reconciles a running environment to a Plan.
type Executor interface {
	Deploy(ctx context.Context, p Plan) (Result, error)
}

// SlotExecutor is an executor whose environment has two slots behind a switchable pointer.
// The engine dispatches slot behavior (switch, slot-rollback) to it by type-assertion.
type SlotExecutor interface {
	Executor                                         // Deploy reconciles the IDLE slot
	Slots() (a, b string)                            // the two slot names
	LiveSlot(ctx context.Context) (string, error)    // slot the pointer routes to; "" if unset
	SwitchTo(ctx context.Context, slot string) error // flip the pointer to slot and reload
}

// FastRollbacker is an executor that can revert to its previous release without a full
// redeploy (a symlink-release executor flips `current` to the prior release dir and runs
// `compose up` without pulling). The engine dispatches --fast rollback to it by type-assertion.
type FastRollbacker interface {
	FastRollback(ctx context.Context) (release string, err error)
}

// RunnerCloser is an executor whose underlying transport can be released between uses. The
// daemon type-asserts to it and calls CloseRunner after each environment's reconcile so a
// long-running process does not leak a pooled SSH connection per cycle (C3).
type RunnerCloser interface {
	CloseRunner() error
}
