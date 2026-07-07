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

func TestRead_DuplicateKeyIsAnError(t *testing.T) {
	_, err := Read(strings.NewReader("SVC_IMAGE=reg/svc:v1\nSVC_IMAGE=reg/svc:v2\n"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate pin key")
}

func TestRead_DistinctKeysParse(t *testing.T) {
	s, err := Read(strings.NewReader("A=1\nB=2\n"))
	require.NoError(t, err)
	require.Equal(t, Set{"A": "1", "B": "2"}, s)
}
