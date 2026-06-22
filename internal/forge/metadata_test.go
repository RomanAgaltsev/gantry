package forge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const sampleBody = "Notes here.\n" +
	"<!-- gantry-release-metadata:v1:start -->\n" +
	"```json\n" +
	`{"schema_version":"1","component":"ses-service","semver_version":"v1.4.0",` +
	`"image_repository":"reg/ses-service","image_tag":"v1.4.0",` +
	`"image_digest":"sha256:abc","commit_sha":"deadbeef",` +
	`"built_at":"2026-06-18T10:00:00Z","changelog_section":"### Added\n- x"}` + "\n" +
	"```\n" +
	"<!-- gantry-release-metadata:v1:end -->\n"

func TestParseMetadata_OK(t *testing.T) {
	r, err := ParseMetadata(sampleBody, "gantry-release-metadata")
	require.NoError(t, err)
	require.Equal(t, "ses-service", r.Component)
	require.Equal(t, "reg/ses-service", r.ImageRepository)
	require.Equal(t, "v1.4.0", r.ImageTag)
	require.Equal(t, "sha256:abc", r.ImageDigest)
	require.Equal(t, time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC), r.BuiltAt)
}

func TestParseMetadata_MissingBlock(t *testing.T) {
	_, err := ParseMetadata("just notes, no block", "gantry-release-metadata")
	require.Error(t, err)
}

func TestParseMetadata_BadJSON(t *testing.T) {
	body := "<!-- gantry-release-metadata:v1:start -->\n```json\n{bad}\n```\n<!-- gantry-release-metadata:v1:end -->"
	_, err := ParseMetadata(body, "gantry-release-metadata")
	require.Error(t, err)
}
