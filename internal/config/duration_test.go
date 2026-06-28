package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDurationUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"14d", 14 * 24 * time.Hour},
		{"72h", 72 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, c := range cases {
		var d Duration
		require.NoError(t, yaml.Unmarshal([]byte(c.in), &d), c.in)
		require.Equal(t, c.want, time.Duration(d), c.in)
	}
}

func TestDurationUnmarshalInvalid(t *testing.T) {
	for _, in := range []string{"banana", "7days", `""`, "d"} {
		var d Duration
		require.Error(t, yaml.Unmarshal([]byte(in), &d), in)
	}
}

func TestThresholdOrDefault(t *testing.T) {
	require.Equal(t, 7*24*time.Hour, DriftConfig{}.ThresholdOrDefault()) // unset → 7d
	require.Equal(t, 3*24*time.Hour, DriftConfig{Threshold: Duration(3 * 24 * time.Hour)}.ThresholdOrDefault())
}
