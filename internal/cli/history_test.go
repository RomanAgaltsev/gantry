package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoryCmd_RequiresEnv(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"history"})
	require.Error(t, cmd.Execute()) // --env is required
}
