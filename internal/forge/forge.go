// Package forge reads component Releases (and their metadata block) from a forge.
package forge

import (
	"context"
	"time"
)

// Component identifies a buildable repo and the pin key its image is written under.
type Component struct {
	ID      string
	Project string // forge project path or numeric id
	PinKey  string
}

// Release is the consumed, parsed form of a component's published release.
type Release struct {
	Component        string
	SemverVersion    string
	ImageRepository  string
	ImageTag         string
	ImageDigest      string
	CommitSHA        string
	ChangelogSection string
	BuiltAt          time.Time
}

// ImageRef is the immutable reference an environment pins. When the release
// metadata carries a digest, it is pinned as "repository:tag@sha256:…" so the
// pulled image cannot drift if the tag is later re-pushed; the readable tag is
// retained alongside the digest. Without a digest it falls back to "repository:tag".
func (r Release) ImageRef() string {
	ref := r.ImageRepository + ":" + r.ImageTag
	if r.ImageDigest != "" {
		ref += "@" + r.ImageDigest
	}
	return ref
}

// Forge reads releases for components.
type Forge interface {
	LatestRelease(ctx context.Context, c Component) (Release, error)
}
