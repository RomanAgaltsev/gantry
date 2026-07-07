package composessh

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHRunner_CloseUndialedIsNoOp(t *testing.T) {
	r := &sshRunner{addr: "h:22"}
	require.NoError(t, r.Close())
	require.NoError(t, r.Close())
}
