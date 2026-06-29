package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

func TestBuildVerifiers(t *testing.T) {
	require.Nil(t, buildVerifiers(nil, nil, config.ExecutorConfig{}))

	probes := []config.VerifyProbe{
		{Kind: "http", URL: "https://app/healthz", ExpectStatus: 200},
		{Kind: "compose-ps"},
		{Kind: "command", Command: "true"},
	}
	v := buildVerifiers(probes, stubRunner{}, config.ExecutorConfig{ProjectDir: "/o", ComposeFiles: []string{"compose.yaml"}, EnvFile: ".env"})
	require.NotNil(t, v)
	comp, ok := v.(verify.Composite)
	require.True(t, ok)
	require.Len(t, comp, 3)
}

type stubRunner struct{}

func (stubRunner) Run(_ context.Context, _ string, _ []byte) (string, error) { return "", nil }
