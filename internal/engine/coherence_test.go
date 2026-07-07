package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func TestOrphansAndMissingKeys(t *testing.T) {
	cfg := &config.Config{Components: []config.Component{
		{ID: "a", PinKey: "A_IMAGE", Project: "g/a"},
		{ID: "b", PinKey: "B_IMAGE", Project: "g/b"},
	}}
	current := pin.Set{"A_IMAGE": "reg/a:v1", "OLD_IMAGE": "reg/x:v0"} // B missing, OLD orphaned

	require.Equal(t, []string{"OLD_IMAGE"}, Orphans(cfg, current))
	require.Equal(t, []string{"B_IMAGE"}, MissingKeys(cfg, current))
}
