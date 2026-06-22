package forge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageRef(t *testing.T) {
	withDigest := Release{ImageRepository: "reg/api", ImageTag: "v1.4.0", ImageDigest: "sha256:abc"}
	require.Equal(t, "reg/api:v1.4.0@sha256:abc", withDigest.ImageRef(),
		"a digest must be pinned alongside the tag for immutability")

	noDigest := Release{ImageRepository: "reg/api", ImageTag: "v1.4.0"}
	require.Equal(t, "reg/api:v1.4.0", noDigest.ImageRef())
}
