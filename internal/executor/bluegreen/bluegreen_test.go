package bluegreen

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type fakeRunner struct {
	cmds        []string
	stdins      [][]byte
	readlink    string
	readlinkErr error
}

func (f *fakeRunner) Run(_ context.Context, cmd string, stdin []byte) (string, error) {
	f.cmds = append(f.cmds, cmd)
	f.stdins = append(f.stdins, stdin)
	if strings.Contains(cmd, "readlink") {
		return f.readlink, f.readlinkErr
	}
	return "", nil
}

func bgExec(fr *fakeRunner) *Executor {
	return &Executor{
		Runner: fr,
		SlotMap: map[string]Slot{
			"blue":  {ProjectDir: "/opt/blue", ComposeFiles: []string{"compose.yaml"}},
			"green": {ProjectDir: "/opt/green", ComposeFiles: []string{"compose.yaml"}},
		},
		Order: [2]string{"blue", "green"},
		Pointer: Pointer{
			Link:   "/etc/nginx/front.conf",
			Target: map[string]string{"blue": "/etc/nginx/blue.conf", "green": "/etc/nginx/green.conf"},
			Reload: "nginx -s reload",
		},
	}
}

func TestDeploy_StagesIdleSlot(t *testing.T) {
	fr := &fakeRunner{readlink: "/etc/nginx/green.conf"} // green live -> idle blue
	res, err := bgExec(fr).Deploy(context.Background(), executor.Plan{Pins: pin.Set{"K": "img:v2"}})
	require.NoError(t, err)
	require.Contains(t, res.Detail, "idle=blue")
	joined := strings.Join(fr.cmds, "\n")
	require.Contains(t, joined, "readlink")
	require.Contains(t, joined, "cat > '/opt/blue/.env'")
	require.Contains(t, joined, "cd '/opt/blue'") // compose runs in the blue slot
	require.NotContains(t, joined, "/opt/green")
}

func TestDeploy_BootstrapStagesBlue(t *testing.T) {
	fr := &fakeRunner{readlink: ""} // no pointer yet (empty probe output) -> bootstrap
	res, err := bgExec(fr).Deploy(context.Background(), executor.Plan{Pins: pin.Set{"K": "img:v1"}})
	require.NoError(t, err)
	require.Contains(t, res.Detail, "idle=blue")
}

func TestDeploy_PointerErrorIsNotMaskedAsBootstrap(t *testing.T) {
	fr := &fakeRunner{readlinkErr: context.DeadlineExceeded} // transport failure, not a missing link
	_, err := bgExec(fr).Deploy(context.Background(), executor.Plan{Pins: pin.Set{"K": "img:v1"}})
	require.Error(t, err) // must surface, not silently stage blue
	require.NotContains(t, strings.Join(fr.cmds, "\n"), "cat >")
}

func TestLiveSlot(t *testing.T) {
	fr := &fakeRunner{readlink: "/etc/nginx/blue.conf"}
	live, err := bgExec(fr).LiveSlot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "blue", live)

	fr2 := &fakeRunner{readlink: ""} // no pointer -> bootstrap
	live, err = bgExec(fr2).LiveSlot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "", live)

	fr3 := &fakeRunner{readlink: "/etc/nginx/unknown.conf"}
	_, err = bgExec(fr3).LiveSlot(context.Background())
	require.Error(t, err) // resolves to neither configured target

	fr4 := &fakeRunner{readlinkErr: context.DeadlineExceeded}
	_, err = bgExec(fr4).LiveSlot(context.Background())
	require.Error(t, err) // a transport error is propagated, not masked as bootstrap
}

func TestSwitchTo(t *testing.T) {
	fr := &fakeRunner{}
	err := bgExec(fr).SwitchTo(context.Background(), "green")
	require.NoError(t, err)
	require.Len(t, fr.cmds, 2) // flip, reload
	require.Contains(t, fr.cmds[0], "ln -sfn '/etc/nginx/green.conf'")
	require.Contains(t, fr.cmds[0], "mv -Tf")
	require.Contains(t, fr.cmds[0], "'/etc/nginx/front.conf'")
	require.Equal(t, "nginx -s reload", fr.cmds[1])
}

func TestSwitchTo_UnknownSlot(t *testing.T) {
	require.Error(t, bgExec(&fakeRunner{}).SwitchTo(context.Background(), "red"))
}

func TestComposeTarget_ResolvesIdleSlot(t *testing.T) {
	fr := &fakeRunner{readlink: "/etc/nginx/green.conf"} // green live -> idle blue
	tgt, err := bgExec(fr).ComposeTarget(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/opt/blue", tgt.ProjectDir) // the idle slot's project
	require.Equal(t, ".env", tgt.EnvFile)
}

func TestComposeTarget_PropagatesPointerError(t *testing.T) {
	fr := &fakeRunner{readlinkErr: context.DeadlineExceeded}
	_, err := bgExec(fr).ComposeTarget(context.Background())
	require.Error(t, err)
}
