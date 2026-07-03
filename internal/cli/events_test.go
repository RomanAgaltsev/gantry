package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

func TestDeployEvents_Success(t *testing.T) {
	evs := deployEvents("test", engine.DeployResult{Deployed: true, Pins: pin.Set{"A": "1", "B": "2"}}, nil)
	require.Len(t, evs, 1)
	require.Equal(t, "deployed", evs[0].Kind)
	require.Contains(t, evs[0].Message, "deployed 2 pin(s) to test")
}

func TestDeployEvents_VerifyFailAndRollback(t *testing.T) {
	evs := deployEvents("prod", engine.DeployResult{VerifyFailed: true, AutoRolledBack: true, RolledBackTo: "1a2b3c4d"}, errors.New("verify failed, rolled back"))
	kinds := []string{evs[0].Kind, evs[1].Kind}
	require.ElementsMatch(t, []string{"verify_failed", "rolled_back"}, kinds)
}

func TestDeployEvents_PlainDeployFailureIsSilent(t *testing.T) {
	evs := deployEvents("prod", engine.DeployResult{}, errors.New("ssh down"))
	require.Empty(t, evs) // not a verify failure -> no event
}

func TestPromoteEvents_Success(t *testing.T) {
	evs := promoteEvents("test", "prod", engine.PromoteResult{Deployed: true, FromSHA: "abcdef1234", Pins: pin.Set{"A": "1"}}, nil)
	require.Len(t, evs, 1)
	require.Equal(t, "promoted", evs[0].Kind)
	require.Contains(t, evs[0].Message, "promoted test@abcdef1 -> prod (1 pins)")
}

func TestRollbackEvents_Success(t *testing.T) {
	evs := rollbackEvents("prod", engine.RollbackResult{Deployed: true, ToSHA: "9f8e7d6c"}, nil)
	require.Len(t, evs, 1)
	require.Equal(t, "rolled_back", evs[0].Kind)
	require.Contains(t, evs[0].Message, "rolled back prod to 9f8e7d6")
}

func TestDriftEvents_OnePerComponent(t *testing.T) {
	rep := engine.DriftReport{Items: []engine.DriftItem{
		{Env: "test", Component: "api"}, {Env: "test", Component: "web"},
	}}
	evs := driftEvents(rep)
	require.Len(t, evs, 2)
	require.Equal(t, "drift_alarm", evs[0].Kind)
}
