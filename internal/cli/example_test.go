package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
)

func TestExampleConfigLoads(t *testing.T) {
	_, err := config.Load("../../examples/demo/gantry.yaml")
	require.NoError(t, err)
}
