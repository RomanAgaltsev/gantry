package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromoteCmd_RequiresFromTo(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"promote"})
	require.Error(t, cmd.Execute()) // --from/--to are required
}
