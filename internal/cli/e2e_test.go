package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/forge"
)

// oldReleaseForge serves a release published past the 7d default drift threshold under a ref
// no committed pin matches, so `drift` must report drift. It stands in for a live forge so the
// CLI's wiring and exit-code mapping can be exercised without HTTP.
type oldReleaseForge struct{}

func (oldReleaseForge) LatestRelease(context.Context, forge.Component) (forge.Release, error) {
	return forge.Release{
		Component:       "svc",
		ImageRepository: "reg/svc",
		ImageTag:        "v9",
		BuiltAt:         time.Now().Add(-10 * 24 * time.Hour), // older than the 7d default threshold
	}, nil
}

// TestCLI_DriftExitCode drives `gantry drift --all` through NewRootCmd with the forge seam
// swapped for oldReleaseForge, pinning the drift → exit-code-3 mapping and the CLI wiring that
// the engine-level e2e test does not cover (review §6 Gap 3).
func TestCLI_DriftExitCode(t *testing.T) {
	t.Setenv("GANTRY_TEST_TOK", "tok")
	old := newForgeFunc
	t.Cleanup(func() { newForgeFunc = old })
	newForgeFunc = func(config.ForgeConfig, string) (forge.Forge, error) { return oldReleaseForge{}, nil }

	// An empty repo → empty pins; a release 10d old differs from the (absent) pin → drift.
	// drift --all uses buildDeps(…, needForge=true, needExec=false), so the file-based SSH
	// creds below are never resolved.
	const cfgYAML = `
forge: { kind: gitlab, base_url: https://x, token: "${env:GANTRY_TEST_TOK}" }
connections:
  h: { address: 10.0.0.1, ssh: { user: deploy, key: "${file:/does/not/exist}", known_hosts: "${file:/does/not/exist}" } }
components:
  - { id: svc, project: g/svc, pin_key: SVC_IMAGE }
environments:
  - name: test
    source: { track: latest }
    pin_file: .env.versions.test
    executor: { kind: compose-over-ssh, connection: h, project_dir: /opt/app, env_file: .env.versions.test }
`
	path := writeTempRepo(t, cfgYAML)

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"drift", "--all", "--config", path})
	err := root.Execute()
	require.ErrorIs(t, err, ErrDriftDetected)
	require.Equal(t, 3, ExitCode(err)) // drift ⇒ exit 3
}
