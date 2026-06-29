package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge/github"
	"github.com/RomanAgaltsev/gantry/internal/forge/gitlab"
)

func TestNewForge(t *testing.T) {
	gl, err := newForge(config.ForgeConfig{Kind: "gitlab", BaseURL: "https://gl"}, "tok")
	require.NoError(t, err)
	require.IsType(t, &gitlab.Client{}, gl)

	gh, err := newForge(config.ForgeConfig{Kind: "github", BaseURL: "https://api.github.com"}, "tok")
	require.NoError(t, err)
	require.IsType(t, &github.Client{}, gh)

	_, err = newForge(config.ForgeConfig{Kind: "bitbucket"}, "tok")
	require.Error(t, err)
}
