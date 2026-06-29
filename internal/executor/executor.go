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
