package forge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPrerelease(t *testing.T) {
	require.False(t, IsPrerelease("1.2.0"))
	require.False(t, IsPrerelease("v1.2.0"))
	require.False(t, IsPrerelease("1.2.0+build.5")) // build metadata is not a prerelease
	require.True(t, IsPrerelease("1.2.0-rc1"))
	require.True(t, IsPrerelease("v2.0.0-beta.1"))
	require.True(t, IsPrerelease("1.0.0-alpha+build"))
	require.False(t, IsPrerelease("")) // empty ⇒ treat as stable (don't skip an unlabeled release)
}
