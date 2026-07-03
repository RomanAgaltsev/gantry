package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

func TestBuildVerifiers(t *testing.T) {
	require.Nil(t, buildVerifiers(nil, nil, fakeComposeExec{}))

	probes := []config.VerifyProbe{
		{Kind: "http", URL: "https://app/healthz", ExpectStatus: 200},
		{Kind: "compose-ps"},
		{Kind: "command", Command: "true"},
	}
	v := buildVerifiers(probes, stubRunner{}, fakeComposeExec{target: verify.ComposeTarget{ProjectDir: "/o", ComposeFiles: []string{"compose.yaml"}, EnvFile: ".env"}})
	require.NotNil(t, v)
	comp, ok := v.(verify.Composite)
	require.True(t, ok)
	require.Len(t, comp, 3)
}

type stubRunner struct{}

func (stubRunner) Run(_ context.Context, _ string, _ []byte) (string, error) { return "", nil }

type fakeComposeExec struct {
	target verify.ComposeTarget
}

func (f fakeComposeExec) Deploy(context.Context, executor.Plan) (executor.Result, error) {
	return executor.Result{}, nil
}
func (f fakeComposeExec) ComposeTarget(context.Context) (verify.ComposeTarget, error) {
	return f.target, nil
}

type recordRunner struct{ gotCmd string }

func (r *recordRunner) Run(_ context.Context, cmd string, _ []byte) (string, error) {
	r.gotCmd = cmd
	return `{"Service":"api","State":"running","Health":"healthy"}`, nil
}

func TestBuildVerifiers_ComposePSUsesExecutorTarget(t *testing.T) {
	runner := &recordRunner{}
	ex := fakeComposeExec{target: verify.ComposeTarget{ProjectDir: "/opt/idle", ComposeFiles: []string{"c.yaml"}, EnvFile: ".env"}}
	vf := buildVerifiers([]config.VerifyProbe{{Kind: "compose-ps"}}, runner, ex)
	require.NoError(t, vf.Verify(context.Background()))
	require.Contains(t, runner.gotCmd, "/opt/idle") // ran against the executor's target
}
