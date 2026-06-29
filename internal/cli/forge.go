package cli

import (
	"fmt"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/forge/github"
	"github.com/RomanAgaltsev/gantry/internal/forge/gitlab"
)

// newForge selects the forge adapter by kind. Adding a forge is adding a case here;
// the engine never learns the kind (XC-4). Unknown kinds are already rejected by
// config validation; the default branch is defense in depth.
func newForge(fc config.ForgeConfig, token string) (forge.Forge, error) {
	switch fc.Kind {
	case "gitlab":
		return gitlab.New(fc.BaseURL, token, fc.MetadataMarker, nil), nil
	case "github":
		return github.New(fc.BaseURL, token, fc.MetadataMarker, nil), nil
	default:
		return nil, fmt.Errorf("unsupported forge.kind %q", fc.Kind)
	}
}
