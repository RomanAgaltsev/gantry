package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRollbackCmd_RequiresEnv(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"rollback"})
	require.Error(t, cmd.Execute()) // --env is required
}
