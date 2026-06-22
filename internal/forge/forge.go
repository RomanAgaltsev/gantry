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

// ImageRef is the immutable reference an environment pins: "repository:tag".
func (r Release) ImageRef() string { return r.ImageRepository + ":" + r.ImageTag }

// Forge reads releases for components.
type Forge interface {
	LatestRelease(ctx context.Context, c Component) (Release, error)
}
