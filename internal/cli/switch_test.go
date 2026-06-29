package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor/bluegreen"
)

func TestNewExecutor_BlueGreen(t *testing.T) {
	env := config.Environment{Executor: config.ExecutorConfig{
		Kind: "blue-green",
		Slots: map[string]config.SlotConfig{
			"blue":  {ProjectDir: "/opt/blue", ComposeFiles: []string{"c.yaml"}},
			"green": {ProjectDir: "/opt/green", ComposeFiles: []string{"c.yaml"}},
		},
		Pointer: config.PointerConfig{Link: "/l", Blue: "/b", Green: "/g", Reload: "r"},
	}}
	ex, err := newExecutor(env, nil, nil)
	require.NoError(t, err)
	bg, ok := ex.(*bluegreen.Executor)
	require.True(t, ok)
	require.Equal(t, "/opt/green", bg.SlotMap["green"].ProjectDir)
	require.Equal(t, "/g", bg.Pointer.Target["green"])
	require.Equal(t, [2]string{"blue", "green"}, bg.Order)
}
