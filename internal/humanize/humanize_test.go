package humanize

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuration(t *testing.T) {
	require.Equal(t, "0h", Duration(30*time.Minute))
	require.Equal(t, "5h", Duration(5*time.Hour))
	require.Equal(t, "1d", Duration(26*time.Hour))
	require.Equal(t, "3d", Duration(72*time.Hour))
}
