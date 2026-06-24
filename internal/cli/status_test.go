package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type statusFakeForge struct{}

func (statusFakeForge) LatestRelease(_ context.Context, c forge.Component) (forge.Release, error) {
	return forge.Release{ImageRepository: "reg/" + c.ID, ImageTag: "v9"}, nil
}

func TestComponentStatusLine_Explicit(t *testing.T) {
	line, err := componentStatusLine(context.Background(),
		config.Component{PinKey: "POSTGRES_IMAGE", Source: config.ComponentSource{Pin: "explicit"}},
		pin.Set{"POSTGRES_IMAGE": "postgres:16.4"}, statusFakeForge{})
	require.NoError(t, err)
	require.Contains(t, line, "postgres:16.4")
	require.Contains(t, line, "latest=(untracked)")
}

func TestComponentStatusLine_ForgeRelease(t *testing.T) {
	line, err := componentStatusLine(context.Background(),
		config.Component{ID: "svc", Project: "g/svc", PinKey: "SVC_IMAGE", Source: config.ComponentSource{Forge: "release"}},
		pin.Set{"SVC_IMAGE": "reg/svc:v1"}, statusFakeForge{})
	require.NoError(t, err)
	require.Contains(t, line, "latest=reg/svc:v9")
}
