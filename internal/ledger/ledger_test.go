package ledger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func sample() []Entry {
	return []Entry{
		{Environment: "test", PinCommit: "aaa", Result: "ok", By: "sync"},
		{Environment: "test", PinCommit: "bbb", Result: "failed", By: "sync"},
		{Environment: "test", PinCommit: "bbb", Result: "ok", By: "sync"}, // retry: latest wins
		{Environment: "prod", PinCommit: "ccc", Result: "ok", By: "promote"},
	}
}

func TestLookup_LatestWins(t *testing.T) {
	e, ok := lookup(sample(), "test", "bbb")
	require.True(t, ok)
	require.Equal(t, "ok", e.Result) // the retry entry, not the earlier failed one
}

func TestLookup_Missing(t *testing.T) {
	_, ok := lookup(sample(), "test", "zzz")
	require.False(t, ok)
}

func TestLatestGreen(t *testing.T) {
	e, ok := latestGreen(sample(), "test")
	require.True(t, ok)
	require.Equal(t, "bbb", e.PinCommit) // most recent ok for test
}

func TestLatestGreen_None(t *testing.T) {
	entries := []Entry{{Environment: "test", PinCommit: "aaa", Result: "failed"}}
	_, ok := latestGreen(entries, "test")
	require.False(t, ok)
}

func TestHistory_NewestFirst(t *testing.T) {
	h := history(sample(), "test")
	require.Len(t, h, 3)
	require.Equal(t, "bbb", h[0].PinCommit) // newest first
	require.Equal(t, "aaa", h[2].PinCommit)
}
