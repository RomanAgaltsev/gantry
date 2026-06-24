package forge

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type metadataJSON struct {
	SchemaVersion    string `json:"schema_version"`
	Component        string `json:"component"`
	SemverVersion    string `json:"semver_version"`
	ImageRepository  string `json:"image_repository"`
	ImageTag         string `json:"image_tag"`
	ImageDigest      string `json:"image_digest"`
	CommitSHA        string `json:"commit_sha"`
	BuiltAt          string `json:"built_at"`
	ChangelogSection string `json:"changelog_section"`
}

// ParseMetadata extracts the "<marker>:v1" JSON block from a release body.
// A missing or invalid block is an error (gantry never silently skips a release).
func ParseMetadata(body, marker string) (Release, error) {
	start := fmt.Sprintf("<!-- %s:v1:start -->", marker)
	end := fmt.Sprintf("<!-- %s:v1:end -->", marker)
	i := strings.Index(body, start)
	j := strings.Index(body, end)
	if i < 0 || j < 0 || j < i {
		return Release{}, fmt.Errorf("metadata block %q not found", marker)
	}
	inner := body[i+len(start) : j]
	inner = strings.TrimSpace(inner)
	inner = strings.TrimPrefix(inner, "```json")
	inner = strings.TrimPrefix(inner, "```")
	inner = strings.TrimSuffix(strings.TrimSpace(inner), "```")
	inner = strings.TrimSpace(inner)

	var m metadataJSON
	if err := json.Unmarshal([]byte(inner), &m); err != nil {
		return Release{}, fmt.Errorf("invalid metadata JSON: %w", err)
	}
	if m.ImageRepository == "" || m.ImageTag == "" {
		return Release{}, errors.New("metadata missing image_repository/image_tag")
	}
	built, err := time.Parse(time.RFC3339, m.BuiltAt)
	if err != nil {
		return Release{}, fmt.Errorf("invalid built_at %q: %w", m.BuiltAt, err)
	}
	return Release{
		Component:        m.Component,
		SemverVersion:    m.SemverVersion,
		ImageRepository:  m.ImageRepository,
		ImageTag:         m.ImageTag,
		ImageDigest:      m.ImageDigest,
		CommitSHA:        m.CommitSHA,
		ChangelogSection: m.ChangelogSection,
		BuiltAt:          built.UTC(),
	}, nil
}
