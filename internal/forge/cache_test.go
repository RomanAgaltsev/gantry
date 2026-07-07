package forge

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type countingForge struct{ n int64 }

func (c *countingForge) LatestRelease(context.Context, Component) (Release, error) {
	atomic.AddInt64(&c.n, 1)
	return Release{ImageRepository: "reg/svc", ImageTag: "v1"}, nil
}

func TestCache_DeduplicatesWithinTTLAndClears(t *testing.T) {
	inner := &countingForge{}
	c := NewCache(inner, time.Minute)
	comp := Component{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE"}

	for range 5 {
		_, err := c.LatestRelease(context.Background(), comp)
		require.NoError(t, err)
	}
	require.Equal(t, int64(1), atomic.LoadInt64(&inner.n), "5 lookups within TTL ⇒ 1 fetch")

	c.Clear()
	_, _ = c.LatestRelease(context.Background(), comp)
	require.Equal(t, int64(2), atomic.LoadInt64(&inner.n), "Clear forces a refetch")
}
