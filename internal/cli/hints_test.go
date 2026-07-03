package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeployFailureHint(t *testing.T) {
	require.Empty(t, deployFailureHint("prod", "")) // nothing committed → no hint

	h := deployFailureHint("prod", "abcdef1234567")
	require.Contains(t, h, "gantry deploy --env prod")
	require.Contains(t, h, "abcdef1") // abbreviated SHA
}

func TestUpToDateMessage(t *testing.T) {
	require.Equal(t, "up to date; no changes", upToDateMessage(false, false))
	require.Equal(t, "recovered: redeployed the last committed pin set", upToDateMessage(true, true))
	// dry-run: recovery was detected but nothing was deployed — must not claim it redeployed
	require.Contains(t, upToDateMessage(false, true), "would redeploy")
}

func TestPromoteDAGWarning(t *testing.T) {
	require.Empty(t, promoteDAGWarning("prod", "", "test"))     // no configured edge → no warning
	require.Empty(t, promoteDAGWarning("prod", "test", "test")) // matches the configured edge

	w := promoteDAGWarning("prod", "test", "stage")
	require.Contains(t, w, "promote_from")
	require.Contains(t, w, "test")
}

func TestAutoRollbackNote(t *testing.T) {
	require.Equal(t, "", autoRollbackNote("prod", ""))
	require.Contains(t, autoRollbackNote("prod", "1a2b3c4d5e"), "rolled back to 1a2b3c4")
	require.Contains(t, autoRollbackNote("front", "blue"), "rolled back to blue")
}
