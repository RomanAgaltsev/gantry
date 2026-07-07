package ledger

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntry_JSONWireFormatUnchanged(t *testing.T) {
	e := Entry{Environment: "prod", PinCommit: "abc", Result: ResultOK, Healthy: HealthTrue}
	b, err := json.Marshal(e)
	require.NoError(t, err)
	require.Contains(t, string(b), `"result":"ok"`)
	require.Contains(t, string(b), `"healthy":"true"`)

	var back Entry
	require.NoError(t, json.Unmarshal(b, &back))
	require.Equal(t, ResultOK, back.Result)
	require.Equal(t, HealthTrue, back.Healthy)
}

func sample() []Entry {
	return []Entry{
		{Environment: "test", PinCommit: "aaa", Result: ResultOK, By: "sync"},
		{Environment: "test", PinCommit: "bbb", Result: ResultFailed, By: "sync"},
		{Environment: "test", PinCommit: "bbb", Result: ResultOK, By: "sync"}, // retry: latest wins
		{Environment: "prod", PinCommit: "ccc", Result: ResultOK, By: "promote"},
	}
}

func TestLookup_LatestWins(t *testing.T) {
	e, ok := lookup(sample(), "test", "bbb")
	require.True(t, ok)
	require.Equal(t, ResultOK, e.Result) // the retry entry, not the earlier failed one
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
	entries := []Entry{{Environment: "test", PinCommit: "aaa", Result: ResultFailed}}
	_, ok := latestGreen(entries, "test")
	require.False(t, ok)
}

func TestHistory_NewestFirst(t *testing.T) {
	h := history(sample(), "test")
	require.Len(t, h, 3)
	require.Equal(t, "bbb", h[0].PinCommit) // newest first
	require.Equal(t, "aaa", h[2].PinCommit)
}
