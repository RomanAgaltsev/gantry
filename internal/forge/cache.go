package forge

import (
	"context"
	"sync"
	"time"
)

// Cache decorates a Forge with a short-TTL, per-component release cache so a daemon cycle
// fetches each component's latest release once instead of once per environment and again for
// drift (review P1). The daemon clears it at the top of each reconcile cycle; a doorbell-rung
// cycle within the TTL reuses the cached snapshot rather than multiplying calls.
type Cache struct {
	inner Forge
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	rel Release
	at  time.Time
}

// NewCache wraps inner with a per-component TTL cache. ttl is the window during which a
// cached release is reused without re-fetching; the daemon sets it to the reconcile interval.
func NewCache(inner Forge, ttl time.Duration) *Cache {
	return &Cache{inner: inner, ttl: ttl, now: time.Now, entries: map[string]cacheEntry{}}
}

// LatestRelease returns the cached release for the component when it is fresh, else fetches
// and caches it. Errors are not cached, so a transient failure is retried on the next call.
func (c *Cache) LatestRelease(ctx context.Context, comp Component) (Release, error) {
	key := comp.Project + "\x00" + comp.PinKey
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.now().Sub(e.at) < c.ttl {
		c.mu.Unlock()
		return e.rel, nil
	}
	c.mu.Unlock()

	rel, err := c.inner.LatestRelease(ctx, comp)
	if err != nil {
		return Release{}, err
	}
	c.mu.Lock()
	c.entries[key] = cacheEntry{rel: rel, at: c.now()}
	c.mu.Unlock()
	return rel, nil
}

// Clear drops all cached releases, forcing the next lookup per component to refetch. The
// daemon calls this at the start of each reconcile cycle so "once per cycle" is exact.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = map[string]cacheEntry{}
	c.mu.Unlock()
}
