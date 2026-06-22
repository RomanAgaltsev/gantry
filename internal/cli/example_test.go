package cli

import (
	"testing"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/stretchr/testify/require"
)

func TestExampleConfigLoads(t *testing.T) {
	_, err := config.Load("../../examples/demo/gantry.yaml")
	require.NoError(t, err)
}
