package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPruneCmd_RequiresEnv(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"prune"})
	require.Error(t, cmd.Execute()) // --env required
}

func TestPruneCmd_Registered(t *testing.T) {
	require.NotNil(t, findCommand(NewRootCmd(), "prune")) // findCommand is defined in serve_test.go
}
