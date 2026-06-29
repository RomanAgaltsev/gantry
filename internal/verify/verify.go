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
