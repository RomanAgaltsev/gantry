package pin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadRenderRoundTrip(t *testing.T) {
	in := "# header\nB_IMAGE=reg/b:v2\n\nA_IMAGE=reg/a:v1\n"
	s, err := Read(strings.NewReader(in))
	require.NoError(t, err)
	require.Equal(t, Set{"A_IMAGE": "reg/a:v1", "B_IMAGE": "reg/b:v2"}, s)
	require.Equal(t, "A_IMAGE=reg/a:v1\nB_IMAGE=reg/b:v2\n", string(Render(s)))
}

func TestDiff(t *testing.T) {
	cur := Set{"A_IMAGE": "reg/a:v1", "B_IMAGE": "reg/b:v2"}
	des := Set{"A_IMAGE": "reg/a:v3", "B_IMAGE": "reg/b:v2", "C_IMAGE": "reg/c:v1"}
	require.Equal(t, []Change{
		{Key: "A_IMAGE", Old: "reg/a:v1", New: "reg/a:v3"},
		{Key: "C_IMAGE", Old: "", New: "reg/c:v1"},
	}, Diff(cur, des))
}

func TestDiff_NoChange(t *testing.T) {
	s := Set{"A_IMAGE": "reg/a:v1"}
	require.Empty(t, Diff(s, s))
}
