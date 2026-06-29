package symlinkrelease

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type fakeRunner struct {
	cmds   []string
	stdins [][]byte
}

func (f *fakeRunner) Run(_ context.Context, cmd string, stdin []byte) (string, error) {
	f.cmds = append(f.cmds, cmd)
	f.stdins = append(f.stdins, stdin)
	return "", nil
}

func TestDeploy_WritesReleaseFlipsThenComposes(t *testing.T) {
	fr := &fakeRunner{}
	e := &Executor{Runner: fr, ProjectDir: "/opt/app", ComposeFiles: []string{"compose.yaml"}}
	res, err := e.Deploy(context.Background(), executor.Plan{
		Env: "prod", PinFile: ".env.versions.prod", Commit: "abc1234",
		Pins: pin.Set{"API_IMAGE": "reg/api:v2"},
	})
	require.NoError(t, err)
	require.True(t, res.Changed)

	// 0: mkdir release dir  1: write .env  2: write .version  3: atomic flip  4: pull  5: up
	require.GreaterOrEqual(t, len(fr.cmds), 6)
	require.Contains(t, fr.cmds[0], "mkdir -p '/opt/app/releases/abc1234'")
	require.Contains(t, fr.cmds[1], "cat > '/opt/app/releases/abc1234/.env'")
	require.Equal(t, "API_IMAGE=reg/api:v2\n", string(fr.stdins[1]))
	require.Contains(t, fr.cmds[2], "/opt/app/releases/abc1234/.version")
	// atomic flip: relative target, temp symlink, mv -T rename over current
	require.Contains(t, fr.cmds[3], "ln -sfn 'releases/abc1234'")
	require.Contains(t, fr.cmds[3], "mv -Tf")
	require.Contains(t, fr.cmds[3], "'/opt/app/current'")
	// compose runs from current/.env
	pullIdx := len(fr.cmds) - 2
	require.Contains(t, fr.cmds[pullIdx], "--env-file 'current/.env'")
	require.True(t, strings.Contains(fr.cmds[pullIdx], "pull"))
	require.True(t, strings.Contains(fr.cmds[pullIdx+1], "up -d"))
}

func TestDeploy_RequiresCommit(t *testing.T) {
	e := &Executor{Runner: &fakeRunner{}, ProjectDir: "/opt/app"}
	_, err := e.Deploy(context.Background(), executor.Plan{Env: "prod", Pins: pin.Set{"K": "v"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "commit")
}
