package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/daemon"
)

func TestServeCmd_Registered(t *testing.T) {
	require.NotNil(t, findCommand(NewRootCmd(), "serve"))
}

func TestMutatingVerbs_RefuseWhileDaemonRuns(t *testing.T) {
	repo := writeTempRepo(t, readOnlyConfig) // path to <dir>/gantry.yaml
	require.NoError(t, os.MkdirAll(filepath.Dir(lockPath(repo)), 0o755))
	l, err := daemon.Acquire(lockPath(repo))
	require.NoError(t, err)
	defer func() { _ = l.Release() }()

	for _, args := range [][]string{
		{"sync", "--env", "test"},
		{"deploy", "--env", "test"},
		{"promote", "--from", "test", "--to", "prod"},
		{"rollback", "--env", "prod"},
		{"switch", "--env", "front"},
	} {
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append(args, "--config", repo))
		err := cmd.Execute()
		require.ErrorContains(t, err, "reconciling this repo", "verb %v should refuse", args)
	}
}

// findCommand returns the named subcommand of root, or nil if absent.
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
