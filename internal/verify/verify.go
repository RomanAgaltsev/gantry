// Package verify runs post-deploy health probes against a freshly-deployed environment.
package verify

import "context"

// Verifier checks that a freshly-deployed environment is healthy. A nil error = healthy.
type Verifier interface {
	Verify(ctx context.Context) error
}

// Composite runs each verifier in order. The first failure wins. An empty Composite
// passes (caller use a nil Verifier to mean "no verification configured").
type Composite []Verifier

// Verify runs every probe. It returns the first probe's error or nil if all pass.
func (c Composite) Verify(ctx context.Context) error {
	for _, v := range c {
		if err := v.Verify(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ComposeTarget describes where an executor runs `docker compose`, so a compose-ps probe can
// health-check the project it actually deployed to.
type ComposeTarget struct {
	ProjectDir   string
	ComposeFiles []string
	EnvFile      string
}

// ComposeVerifiable is implemented by executors whose freshly-deployed compose project can be
// verified with `docker compose ps`. blue-green resolves the idle slot dynamically.
type ComposeVerifiable interface {
	ComposeTarget(ctx context.Context) (ComposeTarget, error)
}
