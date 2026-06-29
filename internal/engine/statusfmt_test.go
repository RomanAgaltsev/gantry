package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sampleMatrix() Matrix {
	return Matrix{
		Components:   []string{"SVC_IMAGE", "PG_IMAGE"},
		Environments: []string{"test", "prod"},
		Latest:       map[string]string{"SVC_IMAGE": "reg/svc:v9", "PG_IMAGE": "(untracked)"},
		Pins: map[string]map[string]string{
			"test": {"SVC_IMAGE": "reg/svc:v9", "PG_IMAGE": "postgres:16.2"},
			"prod": {"SVC_IMAGE": "reg/svc:v8", "PG_IMAGE": "postgres:16.2"},
		},
		Drift: map[string]map[string]bool{
			"test": {"SVC_IMAGE": false, "PG_IMAGE": false},
			"prod": {"SVC_IMAGE": true, "PG_IMAGE": false},
		},
		Health: []EnvHealth{
			{Env: "test", Result: "ok", Healthy: "true", Age: 2 * time.Hour, HasData: true},
			{Env: "prod", HasData: false},
		},
	}
}

func TestFormatMatrix_Content(t *testing.T) {
	out := FormatMatrix(sampleMatrix())

	// Header columns in order.
	require.Regexp(t, `COMPONENT\s+latest\s+test\s+prod`, out)
	// Rows.
	require.Contains(t, out, "reg/svc:v9")
	require.Contains(t, out, "reg/svc:v8 !") // prod cell carries the drift marker
	require.Contains(t, out, "(untracked)")
	require.Contains(t, out, "postgres:16.2")
	// Health footer.
	require.Contains(t, out, "healthy") // test's Healthy "true" rendered friendly
	require.Contains(t, out, "2h ago")  // humanized age
	require.Contains(t, out, "prod  (no deploys)")
}

func TestFormatMatrix_NoMarkerWhenNoDrift(t *testing.T) {
	out := FormatMatrix(sampleMatrix())
	// The test-env SVC cell is up to date: "reg/svc:v9" must not be followed by " !".
	require.NotContains(t, out, "reg/svc:v9 !")
}
