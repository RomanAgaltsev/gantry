package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/ledger"
)

func TestHistory_JSONOutput(t *testing.T) {
	path := writeTempRepo(t, readOnlyConfig)
	led, err := ledger.NewGitLedger(filepath.Dir(path), object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)
	require.NoError(t, led.Record(t.Context(), ledger.Entry{
		Environment: "test", PinCommit: "aaa", Result: "ok", Healthy: "true", DeployedAt: time.Now(),
	}))

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"history", "--env", "test", "--output", "json", "--config", path})
	require.NoError(t, root.Execute())

	var entries []ledger.Entry
	require.NoError(t, json.Unmarshal(out.Bytes(), &entries))
	require.Len(t, entries, 1)
	require.Equal(t, "test", entries[0].Environment)
	require.Equal(t, "ok", string(entries[0].Result))
}

// TestHistory_JSONIsMachineClean ensures that on --output json, stdout is only JSON
// (no human lines mixed in) — the global machine-clean constraint from the plan.
func TestHistory_JSONIsMachineClean(t *testing.T) {
	path := writeTempRepo(t, readOnlyConfig)
	led, err := ledger.NewGitLedger(filepath.Dir(path), object.Signature{Name: "gantry", Email: "gantry@local"})
	require.NoError(t, err)
	require.NoError(t, led.Record(t.Context(), ledger.Entry{
		Environment: "test", PinCommit: "bbb", Result: "failed", Healthy: "false", DeployedAt: time.Now(),
	}))

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetErr(&bytes.Buffer{})
	root.SetOut(&out)
	root.SetArgs([]string{"history", "--env", "test", "-o", "json", "--config", path})
	require.NoError(t, root.Execute())

	// The whole stdout must be valid JSON (nothing else printed to it).
	var entries []ledger.Entry
	require.NoError(t, json.Unmarshal(out.Bytes(), &entries))
}
