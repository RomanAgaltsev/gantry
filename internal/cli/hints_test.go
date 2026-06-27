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

func TestPromoteDAGWarning(t *testing.T) {
	require.Empty(t, promoteDAGWarning("prod", "", "test"))     // no configured edge → no warning
	require.Empty(t, promoteDAGWarning("prod", "test", "test")) // matches the configured edge

	w := promoteDAGWarning("prod", "test", "stage")
	require.Contains(t, w, "promote_from")
	require.Contains(t, w, "test")
}
